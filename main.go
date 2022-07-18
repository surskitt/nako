package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/flowchartsman/retry"
	flags "github.com/jessevdk/go-flags"
	irc "github.com/thoj/go-ircevent"

	"github.com/awesome-gocui/gocui"
)

type Options struct {
	Server    string   `short:"s" long:"server" env:"NAKO_SERVER" required:"true" description:"IRC server:port"`
	Nick      string   `short:"n" long:"nick" env:"NAKO_NICK" required:"true" description:"IRC nick"`
	User      string   `short:"u" long:"user" env:"NAKO_USER" required:"true" description:"IRC user"`
	Password  string   `short:"p" long:"password" env:"NAKO_PASSWORD" description:"IRC password"`
	Channels  []string `short:"c" long:"channels" env:"NAKO_CHANNELS" env-delim:"," required:"true" description:"Channels to join"`
	UseTLS    bool     `short:"T" long:"tls" env:"NAKO_TLS" description:"Connect to irc using tls"`
	Verbose   bool     `short:"v" long:"verbose" env:"NAKO_VERBOSE" description:"Verbose logging"`
	Debug     bool     `short:"d" long:"debug" env:"NAKO_DEBUG" description:"Debug logging"`
	ShowJoins bool     `short:"j" long:"show-joins" env:"NAKO_SHOW_JOINS" description:"Show join and part messages"`
}

func getTime() string {
	t := time.Now()
	ft := t.Format("15:04")

	return fmt.Sprintf("\x1b[1;30m%s\x1b[0m", ft)
}

func genMsgHandler(channel string, g *gocui.Gui) func(event *irc.Event) {
	return func(event *irc.Event) {
		go func(event *irc.Event) {
			if event.Arguments[0] == channel {
				g.Update(func(g *gocui.Gui) error {
					v, err := g.View("chat")
					if err != nil {
						return err
					}

					nick := event.Nick
					if nick == "" {
						nick = event.Source
					}

					fmt.Fprintln(v, fmt.Sprintf("%s %s: %s", getTime(), nick, event.Arguments[1]))

					return nil
				})
			}
		}(event)
	}
}

func genJoinHandler(channel string, g *gocui.Gui) func(event *irc.Event) {
	return func(event *irc.Event) {
		go func(event *irc.Event) {
			if event.Arguments[0] == channel {
				g.Update(func(g *gocui.Gui) error {
					v, err := g.View("chat")
					if err != nil {
						return err
					}

					switch event.Code {
					case "JOIN":
						fmt.Fprintln(v, fmt.Sprintf("%s -> %s joined %s", getTime(), event.Nick, event.Arguments[0]))
					case "QUIT":
						fmt.Fprintln(v, fmt.Sprintf("%s <- %s left %s", getTime(), event.Nick, event.Arguments[0]))
					}

					return nil
				})
			}
		}(event)
	}
}

func genDebugHandler(channel string, g *gocui.Gui) func(event *irc.Event) {
	return func(event *irc.Event) {
		go func(event *irc.Event) {
			// if event.Arguments[0] == channel {
			if true {
				g.Update(func(g *gocui.Gui) error {
					v, err := g.View("chat")
					if err != nil {
						return err
					}

					fmt.Fprintln(v, fmt.Sprintf("%s Code: %s", getTime(), event.Code))
					fmt.Fprintln(v, fmt.Sprintf("%s Raw: %s", getTime(), event.Raw))
					fmt.Fprintln(v, fmt.Sprintf("%s Nick: %s", getTime(), event.Nick))
					fmt.Fprintln(v, fmt.Sprintf("%s Host: %s", getTime(), event.Host))
					fmt.Fprintln(v, fmt.Sprintf("%s Source: %s", getTime(), event.Source))
					fmt.Fprintln(v, fmt.Sprintf("%s User: %s", getTime(), event.User))
					fmt.Fprintln(v, fmt.Sprintf("%s Tags: %s", getTime(), event.Tags))
					fmt.Fprintln(v, fmt.Sprintf("%s Arguments: %s", getTime(), event.Arguments))

					return nil
				})
			}
		}(event)
	}
}

func genLayout(channel string) func(g *gocui.Gui) error {
	return func(g *gocui.Gui) error {
		maxX, maxY := g.Size()

		if v, err := g.SetView("chat", 0, 0, maxX, maxY-2, gocui.TOP); err != nil {
			if !errors.Is(err, gocui.ErrUnknownView) {
				return err
			}

			v.Autoscroll = true
			v.Wrap = true
			v.Frame = false
		}

		if v, err := g.SetView("channel", 0, maxY-2, len(channel)+2, maxY, gocui.TOP); err != nil {
			if !errors.Is(err, gocui.ErrUnknownView) {
				return err
			}

			v.Frame = false
			v.FgColor = gocui.ColorGreen

			fmt.Fprint(v, channel+">")
		}

		if v, err := g.SetView("entry", len(channel)+2, maxY-2, maxX, maxY, gocui.TOP); err != nil {
			if !errors.Is(err, gocui.ErrUnknownView) {
				return err
			}

			v.Frame = false
			v.Editable = true
			v.Wrap = true

			if _, err := g.SetCurrentView("entry"); err != nil {
				return err
			}

			g.Cursor = true
		}

		return nil
	}
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func entrySwitch(g *gocui.Gui, v *gocui.View) error {
	if _, err := g.SetCurrentView("entry"); err != nil {
		return err
	}

	g.Cursor = true

	return nil
}

func chatSwitch(g *gocui.Gui, v *gocui.View) error {
	if _, err := g.SetCurrentView("chat"); err != nil {
		return err
	}

	g.Cursor = false

	return nil
}

func genSendMsg(c *irc.Connection, nick, channel string) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		if v.Buffer() == "" {
			return nil
		}

		msg := v.Buffer() + " "
		c.Privmsg(channel, msg)
		v.Clear()

		g.Update(func(g *gocui.Gui) error {
			v, err := g.View("chat")
			if err != nil {
				return err
			}

			fmt.Fprintln(v, fmt.Sprintf("%s %s: %s", getTime(), nick, msg))

			return nil
		})

		return nil
	}
}

func entryClear(g *gocui.Gui, v *gocui.View) error {
	v.Clear()
	return nil
}

func main() {
	opts := Options{}
	_, err := flags.Parse(&opts)
	if err != nil {
		log.Fatal(err)
	}

	g, err := gocui.NewGui(gocui.OutputNormal, true)
	if err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	g.Highlight = true
	g.SelFgColor = gocui.ColorGreen
	g.SelFrameColor = gocui.ColorGreen

	g.SetManagerFunc(genLayout(opts.Channels[0]))

	irccon := irc.IRC(opts.Nick, opts.User)
	irccon.Debug = opts.Debug

	irccon.UseTLS = opts.UseTLS
	if opts.UseTLS {
		irccon.TLSConfig = &tls.Config{
			ServerName: strings.Split(opts.Server, ":")[0],
			MinVersion: tls.VersionTLS12,
		}
	}

	irccon.Password = opts.Password

	irccon.AddCallback("PRIVMSG", genMsgHandler(opts.Channels[0], g))

	if opts.ShowJoins {
		irccon.AddCallback("JOIN", genJoinHandler(opts.Channels[0], g))
		irccon.AddCallback("PART", genJoinHandler(opts.Channels[0], g))
	}

	if opts.Verbose {
		irccon.AddCallback("*", genDebugHandler(opts.Channels[0], g))
	}

	retrier := retry.NewRetrier(5, 100*time.Millisecond, 5*time.Second)
	err = retrier.Run(func() error {
		return irccon.Connect(opts.Server)
	})
	if err != nil {
		log.Panicln(err)
	}

	irccon.AddCallback("001", func(e *irc.Event) {
		irccon.Join(opts.Channels[0])
	})

	go irccon.Loop()

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		log.Panicln(err)
	}

	if err := g.SetKeybinding("chat", gocui.KeyTab, gocui.ModNone, entrySwitch); err != nil {
		log.Panicln(err)
	}

	if err := g.SetKeybinding("entry", gocui.KeyTab, gocui.ModNone, chatSwitch); err != nil {
		log.Panicln(err)
	}

	if err := g.SetKeybinding("entry", gocui.KeyEnter, gocui.ModNone, genSendMsg(irccon, opts.Nick, opts.Channels[0])); err != nil {
		log.Panicln(err)
	}

	if err := g.SetKeybinding("entry", gocui.KeyCtrlU, gocui.ModNone, entryClear); err != nil {
		log.Panicln(err)
	}

	if err := g.MainLoop(); err != nil && !errors.Is(err, gocui.ErrQuit) {
		log.Panicln(err)
	}
}
