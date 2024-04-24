package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	wc "github.com/ilya2ik/wcwebcamclient_go"
)

func check(e error) {
	if e != nil {
		log.Panic(e)
	}
}

type WCServer struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	VerifyTLS bool   `json:"verifyTLS"`
	Proxy     string `json:"proxy"`
}

type BotConfig struct {
	Database string   `json:"db"`
	BotToken string   `json:"token"`
	Timeout  int      `json:"timeout"`
	Debug    bool     `json:"debug"`
	WCSrv    WCServer `json:"wcserver"`
}

type Listener struct {
	p   *Pool
	bot *tgbotapi.BotAPI
}

func (a Listener) OnSuccessAuth(client *PoolClient) {
	if client != nil {
		page := fmt.Sprintf("Client authorized\n New SID : %s", client.GetWCClient().GetSID())
		msg := tgbotapi.NewMessage(client.GetChatID(), page)
		msg.ParseMode = "HTML"

		a.bot.Send(msg)
	}
}

func (a Listener) OnConnected(client *PoolClient, status wc.ClientStatus) {
	if client != nil {
		page := fmt.Sprintf("Client status changed\n %s", wc.ClientStatusText(status))
		msg := tgbotapi.NewMessage(client.GetChatID(), page)
		msg.ParseMode = "HTML"

		a.bot.Send(msg)
	}
}

func (a Listener) OnAddLog(client *PoolClient, value string) {
	if client != nil {
		msg := tgbotapi.NewMessage(client.GetChatID(), value)
		msg.ParseMode = "HTML"

		a.bot.Send(msg)
	}
}

func (a Listener) OnUpdateDevices(client *PoolClient, devices []map[string]any) {
	if client != nil {
		headers, rows := MapArrayToTable(devices)

		if headers != nil {
			table := FormatTable(headers, rows)

			msg := tgbotapi.NewMessage(client.GetChatID(), table)
			msg.ParseMode = "MarkdownV2"

			a.bot.Send(msg)
		}
	}
}

func SendAuthorized(id int64, un string) tgbotapi.MessageConfig {
	// if already authorized
	msg := tgbotapi.NewMessage(id,
		fmt.Sprintf("<b>%s</b> is already authorized", un))
	msg.ParseMode = "HTML"
	return msg
}

func Authorize(id int64, un string) tgbotapi.MessageConfig {
	msg := tgbotapi.NewMessage(id,
		fmt.Sprintf("<b>%s</b> is not authorized\n"+
			"<a href=\"/authorize\">/authorize</a> to start working with the bot", un))
	msg.ParseMode = "HTML"
	return msg
}

func main() {
	cfgFile, err := os.Open("config.json")
	check(err)
	byteValue, err := io.ReadAll(cfgFile)
	check(err)
	var wcb_cfg BotConfig
	err = json.Unmarshal(byteValue, &wcb_cfg)
	check(err)
	cfgFile.Close()

	cfg := wc.ClientCfgNew()
	if len(wcb_cfg.WCSrv.Host) > 0 {
		cfg.SetHostURL(wcb_cfg.WCSrv.Host)
	} else {
		cfg.SetHostURL("https://127.0.0.1")
	}
	if wcb_cfg.WCSrv.Port > 0 {
		cfg.SetPort(wcb_cfg.WCSrv.Port)
	}
	if len(wcb_cfg.WCSrv.Proxy) > 0 {
		cfg.SetProxy(wcb_cfg.WCSrv.Proxy)
	}
	cfg.SetVerifyTLS(wcb_cfg.WCSrv.VerifyTLS)

	clientpool, err := NewPool(wcb_cfg.Database, cfg)
	check(err)

	bot, err := tgbotapi.NewBotAPI(wcb_cfg.BotToken)
	check(err)

	clientpool.PushListener(Listener{p: clientpool, bot: bot})

	bot.Debug = wcb_cfg.Debug

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = wcb_cfg.Timeout

	stop := make(chan int)
	updates := bot.GetUpdatesChan(u)
	pooltimer := clientpool.GetPoolTimer()
	go func() {
		for wcupd := range pooltimer {
			client := wcupd.Client
			cid := client.GetChatID()
			var msg tgbotapi.MessageConfig
			switch wcupd.Type {
			case Message:
				{
					var table string
					headers, rows := MapToTable(wcupd.Msg.Params)
					if headers != nil {
						table = FormatTable(headers, rows)
					}

					msg = tgbotapi.NewMessage(cid,
						fmt.Sprintf("`%s`\n`%s`\n%s", wcupd.Msg.Device, wcupd.Msg.Msg, table))
					msg.ParseMode = "MarkdownV2"
				}
			case Media:
				{
					msg = tgbotapi.NewMessage(cid,
						fmt.Sprintf("<pre>%s</pre>\nNew media\n<pre>%s</pre>\n"+
							"<a href=\"/getrid_%d\">/getrid_%d</a>",
							wcupd.Rec.Device, wcupd.Rec.Stamp, int64(wcupd.Rec.Rid), int64(wcupd.Rec.Rid)))
					msg.ParseMode = "HTML"
				}
			case MediaData:
				{
					if wcupd.Data.IsEmpty() {
						msg = tgbotapi.NewMessage(cid,
							fmt.Sprintf("Error: No such rid %d", wcupd.Data.GetId()))
						msg.ParseMode = "HTML"
					} else {
						media := tgbotapi.NewPhoto(cid, wcupd.Data)
						bot.Send(media)
					}
				}
			}
			if len(msg.Text) > 0 {
				bot.Send(msg)
			}
		}
	}()

	go func() {
		//initialization
		req := tgbotapi.NewSetMyCommands(
			tgbotapi.BotCommand{Command: "/start", Description: "Start this bot"},
			tgbotapi.BotCommand{Command: "/authorize", Description: "Try to login as the user"},
			tgbotapi.BotCommand{Command: "/settings", Description: "View all settings options"},
			tgbotapi.BotCommand{Command: "/target", Description: "Change the target device"},
			tgbotapi.BotCommand{Command: "/filter", Description: "Change the incoming filter for devices"},
		)

		bot.Send(req)

		for update := range updates {
			if update.Message != nil { // If we got a message
				log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

				tgid := TgUserId{user_id: update.Message.From.ID, chat_id: update.Message.Chat.ID}
				client := clientpool.ByUCID(tgid)

				if client == nil {
					client, err = clientpool.AddCID(tgid,
						update.Message.From.UserName,
						update.Message.From.FirstName,
						update.Message.From.LastName)
					check(err)
				}

				var msg tgbotapi.MessageConfig
				comm, params := ParseCommand(update.Message.Text)

				switch comm {
				case "/start":
					{
						if client.IsAuthorized() {
							msg = SendAuthorized(update.Message.Chat.ID, update.Message.From.UserName)
						} else {
							// if not authorized
							if client.CanAutoAuthorize() {
								clientpool.Authorize(client)
							} else {
								home_page :=
									"<b>Hello, my name is tgTowc_bot</b>\n" +
										"<a href=\"/authorize\">/authorize</a> to start working with the bot"
								msg = tgbotapi.NewMessage(update.Message.Chat.ID, home_page)
								msg.ParseMode = "HTML"
							}
						}
					}
				case "/authorize":
					{
						if client.IsAuthorized() {
							msg = SendAuthorized(update.Message.Chat.ID, update.Message.From.UserName)
						} else {
							page := "Your <b>login</b>:"
							msg = tgbotapi.NewMessage(update.Message.Chat.ID, page)
							msg.ParseMode = "HTML"
							client.SetStatus(StatusWaitLogin)
						}
					}
				case "/target":
					{
						if !client.IsAuthorized() {
							msg = Authorize(update.Message.Chat.ID, update.Message.From.UserName)
						} else {
							page := fmt.Sprintf("Set <b>target</b> device.\nTo get the list of online devices use the\n "+
								"<a href=\"/devices\">/devices</a> request.\n"+
								"To get all messages from all devices use:\n<b>all</b>.\nCurrent target: <b>%s</b>",
								client.GetTarget())

							msg = tgbotapi.NewMessage(update.Message.Chat.ID, page)
							msg.ParseMode = "HTML"
							client.SetStatus(StatusWaitSetTarget)
						}
					}
				case "/filter":
					{
						if !client.IsAuthorized() {
							msg = Authorize(update.Message.Chat.ID, update.Message.From.UserName)
						} else {
							page := fmt.Sprintf("Describe the incoming message filter by device name.\n"+
								"The filter is set by the regexp expression.\nCurrent filter: <b>%s</b>",
								client.GetFilter())

							msg = tgbotapi.NewMessage(update.Message.Chat.ID, page)
							msg.ParseMode = "HTML"
							client.SetStatus(StatusWaitSetFilter)
						}
					}
				case "/devices":
					{
						if !client.IsAuthorized() {
							msg = Authorize(update.Message.Chat.ID, update.Message.From.UserName)
						} else {
							clientpool.UpdateDevices(client)
						}
					}
				case "/getrid":
					{
						if len(params) > 0 {
							rid, err := strconv.ParseInt(params[0], 10, 64)
							if err == nil {
								clientpool.DownloadRid(client, rid)
							}
						}
					}
				default:
					{
						switch client.GetStatus() {
						case StatusWaitLogin:
							{
								client.SetLogin(update.Message.Text)
								page := "Your <b>password</b>:"
								msg = tgbotapi.NewMessage(update.Message.Chat.ID, page)
								msg.ParseMode = "HTML"
								client.SetStatus(StatusWaitPassword)
							}
						case StatusWaitPassword:
							{
								client.SetPwd(update.Message.Text)
								clientpool.Authorize(client)
							}
						case StatusWaitSetTarget:
							{
								err = clientpool.SetClientTarget(client, update.Message.Text)
								check(err)
								client.SetStatus(StatusAuthorized)
							}
						case StatusWaitSetFilter:
							{
								err = clientpool.SetClientFilter(client, update.Message.Text)
								check(err)
								client.SetStatus(StatusAuthorized)
							}
						default:
							{
								msg = tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
							}
						}
					}
				}
				if len(msg.Text) > 0 {
					msg.ReplyToMessageID = update.Message.MessageID

					bot.Send(msg)
				}
			}
		}
		stop <- 1
	}()

	for loop := true; loop; {
		select {
		case <-stop:
			{
				loop = false
				close(stop)
				break
			}
		default:
			time.Sleep(250 * time.Millisecond)
		}
	}

}
