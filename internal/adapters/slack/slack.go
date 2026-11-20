// Package slack connects KubeChat to Slack over Socket Mode: the bot dials out
// to Slack, so it needs no public URL.
//
// An @mention starts a conversation in a thread. After that, every reply in
// the thread is a question too, with or without a mention, and the agent sees
// the thread's earlier questions and answers.
package slack

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/sskrishnav/kubechat/internal/core"
	"github.com/sskrishnav/kubechat/internal/redact"
)

const (
	maxAnswer     = 3500             // bytes; well under Slack's message limit
	maxAnswerTime = 10 * time.Minute // per question
	maxRemembered = 20               // messages kept per thread: the last 10 questions and answers
)

var mention = regexp.MustCompile(`<@[^>]+>`) // "<@U0123ABCD>" in the message text

type Bot struct {
	AppToken string // xapp-…: the Socket Mode connection
	BotToken string // xoxb-…: posting replies
	// Ask is the agent. earlier is the thread's conversation so far.
	Ask func(ctx context.Context, earlier []core.Message, question string) (string, error)

	mu      sync.Mutex
	threads map[string]*thread // by thread timestamp; in memory, so lost on restart
}

// thread is one conversation.
type thread struct {
	busy    bool           // a question is being answered; one at a time
	history []core.Message // earlier questions and answers, oldest first
}

// question is one message for the bot.
type question struct {
	channel, thread, user, text string
	mentioned                   bool // an @mention may start a conversation; a plain reply may not
}

// Run connects to Slack and answers questions until ctx ends.
func (b *Bot) Run(ctx context.Context) error {
	api := slack.New(b.BotToken, slack.OptionAppLevelToken(b.AppToken))
	me, err := api.AuthTestContext(ctx) // the bot's own user ID, to skip its own messages
	if err != nil {
		return fmt.Errorf("check the Slack bot token: %w", err)
	}
	b.threads = map[string]*thread{}
	client := socketmode.New(api)

	go func() {
		for evt := range client.Events {
			switch evt.Type {
			case socketmode.EventTypeConnected:
				slog.Info("connected to Slack", "bot_user", me.UserID)
			case socketmode.EventTypeEventsAPI:
				client.Ack(*evt.Request) // within 3 seconds, or Slack sends the event again
				event, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				switch m := event.InnerEvent.Data.(type) {
				case *slackevents.AppMentionEvent:
					ts := m.ThreadTimeStamp
					if ts == "" {
						ts = m.TimeStamp // a mention outside a thread starts one
					}
					go b.answer(ctx, api, question{m.Channel, ts, m.User, m.Text, true})
				case *slackevents.MessageEvent:
					// Only plain replies in threads. Mentions arrive as AppMentionEvent too,
					// and the bot's own messages must not trigger it.
					if m.ThreadTimeStamp == "" || m.SubType != "" || m.BotID != "" || m.User == me.UserID ||
						strings.Contains(m.Text, "<@"+me.UserID+">") {
						continue
					}
					go b.answer(ctx, api, question{m.Channel, m.ThreadTimeStamp, m.User, m.Text, false})
				}
			}
		}
	}()
	return client.RunContext(ctx)
}

// answer runs one question and posts the replies in its thread.
func (b *Bot) answer(ctx context.Context, api *slack.Client, q question) {
	reply := func(text string) {
		_, _, err := api.PostMessageContext(ctx, q.channel, slack.MsgOptionText(text, false), slack.MsgOptionTS(q.thread))
		if err != nil {
			slog.Error("post to Slack", "err", err)
		}
	}

	text := strings.TrimSpace(mention.ReplaceAllString(q.text, ""))
	if text == "" {
		if q.mentioned {
			reply("Ask me about the cluster, for example: why are pods failing in apps?")
		}
		return
	}

	// Claim the thread.
	b.mu.Lock()
	t := b.threads[q.thread]
	switch {
	case t == nil && !q.mentioned:
		b.mu.Unlock()
		return // a reply in a thread the bot isn't part of
	case t == nil:
		t = &thread{}
		b.threads[q.thread] = t
	case t.busy:
		b.mu.Unlock()
		reply("I'm still working on the last question in this thread.")
		return
	}
	t.busy = true
	earlier := slices.Clone(t.history)
	b.mu.Unlock()

	slog.Info("question from Slack", "user", q.user, "channel", q.channel, "thread", q.thread,
		"earlier_messages", len(earlier), "question", text)
	reply("Looking into it…")

	askCtx, cancel := context.WithTimeout(ctx, maxAnswerTime) // reply keeps ctx, so a timeout can still be reported
	defer cancel()
	answer, err := b.Ask(askCtx, earlier, text)
	if err != nil {
		slog.Error("no answer", "err", err)
		answer = "Sorry, I couldn't answer that: " + err.Error()
	}
	answer = redact.Clean(answer, maxAnswer)
	reply(answer)

	// Release the thread and remember the exchange, failures too, so "try again" works.
	b.mu.Lock()
	t.busy = false
	t.history = append(t.history,
		core.Message{Role: "user", Text: text},
		core.Message{Role: "assistant", Text: answer})
	if n := len(t.history); n > maxRemembered {
		t.history = t.history[n-maxRemembered:]
	}
	b.mu.Unlock()
}
