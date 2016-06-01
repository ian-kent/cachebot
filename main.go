package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ian-kent/gofigure"
	"github.com/nlopes/slack"
)

// support , separated env vars for URL_BASES and URL_SUFFIXES
var _ = os.Setenv("GOFIGURE_ENV_ARRAY", "1")

type config struct {
	CloudflareToken    string   `env:"CF_TOKEN"`
	CloudflareEmail    string   `env:"CF_EMAIL"`
	CloudflareZone     string   `env:"CF_ZONE"`
	SlackToken         string   `env:"SLACK_TOKEN"`
	URLBases           []string `env:"URL_BASES"`
	URLSuffixes        []string `env:"URL_SUFFIXES"`
	RestrictedChannels []string `env:"RESTRICTED_CHANNELS"`
	AuthorisedUsers    []string `env:"AUTHORISED_USERS"`
}

type cacheClearInput struct {
	URI string `json:"uri,omitempty"`
}

type cloudflareRequest struct {
	PurgeEverything bool     `json:"purge_everything,omitempty"`
	Files           []string `json:"files,omitempty"`
}

type cloudflareResponse struct {
	Success bool `json:"success"`
	// these definitions might be wrong
	Errors   []interface{}          `json:"errors"`
	Messages []interface{}          `json:"messages"`
	Result   map[string]interface{} `json:"result"`
}

type cacheClearPending struct {
	Everything bool
	URIs       []string
	Created    time.Time
	User       string
	Channel    string
}

var clearPending = make(map[string]cacheClearPending)
var clearWaiting []cacheClearPending

var helpMessage = "Here's some examples of how to clear the cache:\n`clear cache`\n`clear cache for /some/uri`\n`clear cache for /some/uri and /another/uri`\nIf I ask you to confirm, reply with `yes` or `no`!"

var cacheQueue = make(chan cacheClearPending, 10)
var wg sync.WaitGroup
var cfg = config{}

var api *slack.Client
var botUserID string
var restrictedChannels []string
var authorisedUsers []string

func main() {
	err := gofigure.Gofigure(&cfg)
	if err != nil {
		panic(err)
	}

	wg.Add(2)
	go slackBot()
	go cacheDeleter()
	wg.Wait()
}

func cacheDeleter() {
	defer wg.Done()

	t := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-t.C:
			fmt.Println("tick")
			if len(clearWaiting) > 0 {
				fmt.Println("found items in waiting")
				for _, v := range clearWaiting {
					err := v.do()
					if err != nil {
						log.Printf("cacheDeleter: Error received: %s\n", err.Error())
						api.PostMessage(v.Channel, "<@"+v.User+"> Sorry, that didn't work...\n*Error*: "+err.Error(), slack.PostMessageParameters{AsUser: true})
						continue
					}
					log.Println("cacheDeleter: 'do' completed without errors")
					if len(v.URIs) > 0 {
						f := strings.Join(v.URIs, "`\n`")
						f = "`" + f + "`"
						api.PostMessage(v.Channel, "<@"+v.User+"> That's done, the following items have been cleared:\n"+f, slack.PostMessageParameters{AsUser: true})
					} else {
						api.PostMessage(v.Channel, "<@"+v.User+"> That's done, the entire cache has been cleared", slack.PostMessageParameters{AsUser: true})
					}
				}
				clearWaiting = make([]cacheClearPending, 0)
			}
		case q := <-cacheQueue:
			fmt.Println("adding item to clearWaiting queue")
			clearWaiting = append(clearWaiting, q)
		}
	}
}

func filesFromURI(uri string) (res []string) {
	uri = strings.TrimSuffix(uri, ",") // supports "clear cache for a, b and c"
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	for _, b := range cfg.URLBases {
		uri = strings.TrimPrefix(uri, b)
	}
	for _, b := range cfg.URLBases {
		res = append(res, b+uri)
		for _, suffix := range cfg.URLSuffixes {
			if !strings.HasSuffix(uri, "/") {
				res = append(res, b+uri+"/"+suffix)
			} else {
				res = append(res, b+uri+suffix)
			}
		}
	}
	return
}

func (c cacheClearPending) do() error {
	var cfReq cloudflareRequest

	if c.Everything {
		log.Println("cacheClearPending [do]: Clearing everything")
		cfReq.PurgeEverything = true
	} else {
		log.Printf("cacheClearPending [do]: Clearing %d files\n", len(c.URIs))
		cfReq.Files = c.URIs
	}

	b, err := json.Marshal(&cfReq)
	if err != nil {
		return fmt.Errorf("Error preparing request for CloudFlare: %s", err)
	}

	cfR, err := http.NewRequest("DELETE", "https://api.cloudflare.com/client/v4/zones/"+cfg.CloudflareZone+"/purge_cache", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("Error creating request for CloudFlare: %s", err)
	}

	cfR.Header.Set("X-Auth-Email", cfg.CloudflareEmail)
	cfR.Header.Set("X-Auth-Key", cfg.CloudflareToken)
	cfR.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(cfR)
	if err != nil {
		return fmt.Errorf("Error sending request to CloudFlare: %s", err)
	}

	defer res.Body.Close()
	b, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("Error reading response from CloudFlare: %s", err)
	}

	var cfRes cloudflareResponse
	err = json.Unmarshal(b, &cfRes)
	if err != nil {
		return fmt.Errorf("Error parsing response from CloudFlare: %s", err)
	}

	if !cfRes.Success {
		log.Printf("%+v\n", cfRes)
		return fmt.Errorf("CloudFlare returned an unsuccessful response")
	}

	log.Println("cacheClearPending [do]: Completed without errors")

	return nil
}

func slackBot() {
	defer wg.Done()
	api = slack.New(cfg.SlackToken)

	a, err := api.AuthTest()
	if err != nil {
		panic(err)
	}

	c, err := api.GetChannels(true)
	if err != nil {
		panic(err)
	}

	botUserID = a.UserID

	rtm := api.NewRTM()
	go rtm.ManageConnection()

	for _, channel := range c {
		api.PostMessage(channel.ID, "I'm ready! Say `help` for more information.", slack.PostMessageParameters{AsUser: true})
		for _, r := range cfg.RestrictedChannels {
			if channel.Name == r {
				restrictedChannels = append(restrictedChannels, channel.ID)
			}
		}
	}

	u, err := api.GetUsers()
	for _, user := range u {
		for _, a := range cfg.AuthorisedUsers {
			if user.Name == a {
				authorisedUsers = append(authorisedUsers, user.ID)
			}
		}
	}

Loop:
	for {
		select {
		case msg := <-rtm.IncomingEvents:
			fmt.Print("Event Received: ")
			switch ev := msg.Data.(type) {
			case *slack.HelloEvent:
				// Ignore hello
			case *slack.ConnectedEvent:
				fmt.Println("Infos:", ev.Info)
				fmt.Println("Connection counter:", ev.ConnectionCount)

			case *slack.MessageEvent:
				// ignore cachebot user
				if ev.User == botUserID {
					continue
				}

				fmt.Printf("Message: %v\n", ev)

				var restricted bool
				for _, r := range restrictedChannels {
					if r == ev.Channel {
						restricted = true
						break
					}
				}
				var authorised bool
				for _, a := range authorisedUsers {
					if a == ev.User {
						authorised = true
						break
					}
				}

				if restricted && !authorised {
					api.PostMessage(ev.Channel, "<@"+ev.User+"> Sorry, cachebot is restricted to authorised users", slack.PostMessageParameters{AsUser: true})
					continue
				}

				switch strings.ToLower(ev.Text) {
				case "help":
					api.PostMessage(ev.Channel, "<@"+ev.User+"> "+helpMessage, slack.PostMessageParameters{AsUser: true})
					continue
				case "yes":
					if _, ok := clearPending[ev.User]; ok {
						api.PostMessage(ev.Channel, "<@"+ev.User+"> Ok, I'll let you know when it's done.", slack.PostMessageParameters{AsUser: true})
						cacheQueue <- clearPending[ev.User]
						delete(clearPending, ev.User)
					}
					continue
				case "no":
					if _, ok := clearPending[ev.User]; ok {
						api.PostMessage(ev.Channel, "<@"+ev.User+"> Ok, I'll cancel that!", slack.PostMessageParameters{AsUser: true})
						delete(clearPending, ev.User)
					}
					continue
				}

				re := regexp.MustCompile("(?:https?:\\/\\/[^\\/]+)?(\\/[^\\s>]*)")
				if strings.Contains(ev.Text, "clear cache") {
					m := re.FindAllStringSubmatch(ev.Text, -1)
					fmt.Printf("Matches: %+v\n", m)

					if len(m) == 0 {
						api.PostMessage(ev.Channel, "<@"+ev.User+"> I'm about to clear the entire cache, are you sure?\n*Warning*: This will cause a spike in traffic to the production environment!", slack.PostMessageParameters{AsUser: true})
						clearPending[ev.User] = cacheClearPending{Everything: true, Created: time.Now(), User: ev.User, Channel: ev.Channel}
						continue
					}

					var uris []string
					for _, match := range m {
						if len(match) > 1 {
							fmt.Printf("Clearing cache: %s\n", match[1])
							uris = append(uris, filesFromURI(match[1])...)
						}
					}

					if len(uris) > 30 {
						api.PostMessage(ev.Channel, "<@"+ev.User+"> That's too much for one request - try again with less URIs", slack.PostMessageParameters{AsUser: true})
						continue
					}

					f := strings.Join(uris, "`\n`")
					f = "`" + f + "`"
					api.PostMessage(ev.Channel, "<@"+ev.User+"> I'm about to clear the following cache items, are you sure?\n"+f, slack.PostMessageParameters{AsUser: true})
					clearPending[ev.User] = cacheClearPending{Everything: true, Created: time.Now(), URIs: uris, User: ev.User, Channel: ev.Channel}
					continue
				}

			case *slack.PresenceChangeEvent:
				fmt.Printf("Presence Change: %v\n", ev)

			case *slack.LatencyReport:
				fmt.Printf("Current latency: %v\n", ev.Value)

			case *slack.RTMError:
				fmt.Printf("Error: %s\n", ev.Error())

			case *slack.InvalidAuthEvent:
				fmt.Printf("Invalid credentials")
				break Loop

			default:

				// Ignore other events..
				fmt.Printf("Unexpected: %v\n", msg.Data)
			}
		}
	}
}
