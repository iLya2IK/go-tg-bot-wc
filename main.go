/*===============================================================*/
/* The Telegram to WCClient Bot                                  */
/*                                                               */
/* Copyright 2024 Ilya Medvedkov                                 */
/*===============================================================*/

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	wc "github.com/ilya2ik/wcwebcamclient_go"
)

func check(e error) {
	if e != nil {
		log.Panic(e)
	}
}

const PM_HTML = "HTML"
const PM_MD2 = "MarkdownV2"

type WCServer struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	VerifyTLS bool   `json:"verifyTLS"`
	Proxy     string `json:"proxy"`
}

type BotConfig struct {
	Database     string   `json:"db"`
	BotToken     string   `json:"token"`
	Timeout      int      `json:"timeout"`
	Debug        bool     `json:"debug"`
	MaxMediaSize int      `json:"max_media_size"`
	WCSrv        WCServer `json:"wcserver"`
}

type Listener struct {
	p   *Pool
	bot *tgbotapi.BotAPI
}

func (a Listener) OnSuccessAuth(client *PoolClient) {
	if client != nil {
		page := fmt.Sprintf(client.GetLocale().ClientAuthorized, client.GetWCClient().GetSID())
		msg := tgbotapi.NewMessage(client.GetChatID(), page)
		msg.ParseMode = PM_HTML

		a.bot.Send(msg)

		SendWorkingCommands(a.bot, client.GetID(), client.GetLocale())
	}
}

func (a Listener) OnConnected(client *PoolClient, status wc.ClientStatus) {
	if client != nil {
		page := fmt.Sprintf(client.GetLocale().ClientStatusChanged, wc.ClientStatusText(status))
		msg := tgbotapi.NewMessage(client.GetChatID(), page)
		msg.ParseMode = PM_HTML

		a.bot.Send(msg)

		if status == wc.StateDisconnected {
			SendInitCommands(a.bot, client.GetID(), client.GetLocale())
		}
	}
}

func DoLog(bot *tgbotapi.BotAPI, chatid int64, value string) {
	msg := tgbotapi.NewMessage(chatid, value)
	msg.ParseMode = PM_HTML

	bot.Send(msg)
}

func (a Listener) OnAddLog(client *PoolClient, value string) {
	if client != nil {
		DoLog(a.bot, client.GetChatID(), value)
	}
}

func (a Listener) OnUpdateDevices(client *PoolClient, devices []map[string]any) {
	if client != nil {
		headers, rows := MapArrayToTable(devices)
		buttons := make([]tgbotapi.InlineKeyboardButton, 0, len(devices))

		for _, dev := range devices {
			name := fmt.Sprintf("%v", dev[wc.JSON_RPC_DEVICE])
			if strings.Compare(name, client.GetUserName()) != 0 {
				btn := tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf(client.GetLocale().SetTarget, name),
					fmt.Sprintf("%s&%s", TG_COMMAND_TARG, name))
				buttons = append(buttons, btn)
			}
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(
			client.GetLocale().SetTargetAll,
			fmt.Sprintf("%s&%s", TG_COMMAND_TARG, ALL_DEVICES))
		buttons = append(buttons, btn)

		if headers != nil {
			table := FormatTable(headers, rows)

			msg := tgbotapi.NewMessage(client.GetChatID(), "<pre>"+table+"</pre>")
			msg.ParseMode = PM_HTML
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons)

			a.bot.Send(msg)
		}
	}
}

func SendAuthorized(id int64, un string, locale *LanguageStrings) tgbotapi.MessageConfig {
	// if already authorized
	msg := tgbotapi.NewMessage(id,
		fmt.Sprintf(locale.AlreadyAuthorized, un))
	msg.ParseMode = PM_HTML
	return msg
}

func SendToAuthorize(id int64, un string, locale *LanguageStrings) tgbotapi.MessageConfig {
	msg := tgbotapi.NewMessage(id,
		fmt.Sprintf(locale.NotAuthorized, un, TG_COMMAND_AUTH, TG_COMMAND_AUTH))
	msg.ParseMode = PM_HTML
	return msg
}

func SetTarget(clientpool *Pool, client *PoolClient, trg string) tgbotapi.MessageConfig {
	err := clientpool.SetClientTarget(client, trg)
	check(err)
	msg := tgbotapi.NewMessage(client.GetChatID(),
		fmt.Sprintf(client.GetLocale().TargetUpdated, trg))
	return msg
}

const TG_COMMAND_START = "/start"
const TG_COMMAND_LOGOUT = "/logout"
const TG_COMMAND_AUTH = "/authorize"
const TG_COMMAND_SETTS = "/settings"
const TG_COMMAND_TARG = "/target"
const TG_COMMAND_FILTER = "/filter"
const TG_COMMAND_DEVS = "/devices"
const TG_COMMAND_GETRID = "/getrid"
const TG_COMMAND_SEND = "/send"
const TG_COMMAND_SEND_ALL = "/sendall"
const TG_COMMAND_ADD_PARAM = "/paramadd"
const TG_COMMAND_DEL_PARAM = "/paramdel"
const TG_COMMAND_EDIT_PARAM = "/paramedit"
const TG_COMMAND_TOJSON = "/tojson"

func SendInitCommands(bot *tgbotapi.BotAPI, tid TgUserId, locale *LanguageStrings) {
	req := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: TG_COMMAND_START, Description: locale.CommandStart},
		tgbotapi.BotCommand{Command: TG_COMMAND_AUTH, Description: locale.CommandAuth},
		tgbotapi.BotCommand{Command: TG_COMMAND_SETTS, Description: locale.CommandSett},
	)
	if tid.chat_id != 0 {
		req.Scope = &tgbotapi.BotCommandScope{Type: "chat", ChatID: tid.chat_id} //, UserID: tid.user_id}
	} else {
		req.Scope = &tgbotapi.BotCommandScope{Type: "all_private_chats"}
	}
	bot.Send(req)
}

func SendWorkingCommands(bot *tgbotapi.BotAPI, tid TgUserId, locale *LanguageStrings) {
	req := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: TG_COMMAND_LOGOUT, Description: locale.CommandLogOut},
		tgbotapi.BotCommand{Command: TG_COMMAND_SETTS, Description: locale.CommandSett},
		tgbotapi.BotCommand{Command: TG_COMMAND_TARG, Description: locale.CommandTarget},
		tgbotapi.BotCommand{Command: TG_COMMAND_FILTER, Description: locale.CommandFilter},
		tgbotapi.BotCommand{Command: TG_COMMAND_DEVS, Description: locale.CommandDevs},
	)
	req.Scope = &tgbotapi.BotCommandScope{Type: "chat", ChatID: tid.chat_id} //, UserID: tid.user_id}
	bot.Send(req)
}

func RebuildMessageEditor(msg *wc.OutMessageStruct, client *PoolClient) (string, tgbotapi.InlineKeyboardMarkup, error) {
	txt, err := GenJSONMessage(msg)

	if err != nil {
		return "", tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{}), err
	}

	var buttons [][]tgbotapi.InlineKeyboardButton

	add := tgbotapi.NewInlineKeyboardButtonData(client.GetLocale().AddParam, TG_COMMAND_ADD_PARAM)
	buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(add))
	if msg.Params != nil {
		for param, v := range msg.Params {
			str := string(TG_COMMAND_EDIT_PARAM + " " + fmt.Sprintf("%v", v))
			edit := tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf(client.GetLocale().EditParam, param),
				TG_COMMAND_EDIT_PARAM+"&"+param)
			edit.SwitchInlineQueryCurrentChat = &str
			buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
				edit,
				tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf(client.GetLocale().DeleteParam, param),
					TG_COMMAND_DEL_PARAM+"&"+param)))
		}
	}
	buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf(client.GetLocale().SendMsgTo, client.GetTarget()),
			TG_COMMAND_SEND)))

	return txt, tgbotapi.InlineKeyboardMarkup{InlineKeyboard: buttons}, nil

}

func GetRid(clientpool *Pool, client *PoolClient, params []string) {
	if len(params) > 0 {
		rid, err := strconv.ParseInt(params[0], 10, 64)
		if err == nil {
			clientpool.DownloadRid(client, rid)
		}
	}
}

func GenJSONMessage(msg *wc.OutMessageStruct) (string, error) {
	msg_map := make(map[string]any)
	msg_map[wc.JSON_RPC_MSG] = msg.Msg
	if msg.Params != nil && len(msg.Params) > 0 {
		msg_map[wc.JSON_RPC_PARAMS] = msg.Params
	}
	b, err := json.MarshalIndent(msg_map, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func GenJSONMessageInline(msg *wc.OutMessageStruct) (string, error) {
	msg_map := make(map[string]any)
	msg_map[wc.JSON_RPC_MSG] = msg.Msg
	if msg.Params != nil && len(msg.Params) > 0 {
		msg_map[wc.JSON_RPC_PARAMS] = msg.Params
	}
	b, err := json.Marshal(msg_map)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func DecodeJSONMessage(msg string) (*wc.OutMessageStruct, error) {
	jsonmap := make(map[string]any)
	err := json.Unmarshal([]byte(msg), &jsonmap)
	if err != nil {
		return nil, err
	}

	type jsonField struct {
		name string
		tp   reflect.Kind
	}

	decl := []jsonField{
		{name: wc.JSON_RPC_TARGET, tp: reflect.String},
		{name: wc.JSON_RPC_MSG, tp: reflect.String},
		{name: wc.JSON_RPC_PARAMS, tp: reflect.Map},
	}

	mr := &wc.OutMessageStruct{}

	for num, v := range decl {
		_any, ok := jsonmap[v.name]
		if ok {
			dt := reflect.TypeOf(_any).Kind()
			if dt == v.tp {
				switch num {
				case 0:
					mr.Target, _ = _any.(string)
				case 1:
					mr.Msg, _ = _any.(string)
				case 2:
					mr.Params, _ = _any.(map[string]any)
				}
			} else {
				return nil, wc.ThrowErrMalformedResponse(wc.EMKWrongType, v.name, fmt.Sprintf("%v", v.tp))
			}
		}
	}

	return mr, nil
}

func FormatOutputMessage(txt string) string {
	return fmt.Sprintf("<pre><code class=\"language-json\">%s</code></pre>", txt)
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

	if wcb_cfg.MaxMediaSize > 0x200000 {
		wcb_cfg.MaxMediaSize = 0x200000
	}
	if wcb_cfg.MaxMediaSize < 0x800 {
		wcb_cfg.MaxMediaSize = 0x800
	}

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
			switch wcupd.Type {
			case Message:
				{
					var table string
					headers, rows := MapToTable(wcupd.Msg.Params)
					if headers != nil {
						table = FormatTable(headers, rows)
					}

					json_text, _ := GenJSONMessageInline(
						&wc.OutMessageStruct{
							Msg:    wcupd.Msg.Msg,
							Params: wcupd.Msg.Params,
						})

					msg := tgbotapi.NewMessage(cid,
						fmt.Sprintf("<pre>%s</pre>\n%s\n<pre>%s</pre>\n<pre>%s</pre>",
							wcupd.Msg.Device, client.GetLocale().IncomingMsg,
							wcupd.Msg.Msg, table))
					msg.ParseMode = PM_HTML
					msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
						tgbotapi.NewInlineKeyboardButtonData(
							client.GetLocale().ToJSON,
							TG_COMMAND_TOJSON+"_"+json_text),
					})
					bot.Send(msg)
				}
			case Media:
				{
					if strings.Compare(wcupd.Rec.Device, client.GetUserName()) != 0 {
						msg := tgbotapi.NewMessage(cid,
							fmt.Sprintf("<pre>%s</pre>\n%s\n<pre>%s</pre>\n"+
								"<a href=\"%s_%d\">%s_%d</a>",
								wcupd.Rec.Device, client.GetLocale().IncomingMedia,
								wcupd.Rec.Stamp,
								TG_COMMAND_GETRID, int64(wcupd.Rec.Rid),
								TG_COMMAND_GETRID, int64(wcupd.Rec.Rid)))
						msg.ParseMode = PM_HTML
						msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardButtonData(client.GetLocale().GetMedia,
								fmt.Sprintf("%s_%d", TG_COMMAND_GETRID, int64(wcupd.Rec.Rid))),
						})
						bot.Send(msg)
					}
				}
			case MediaData:
				{
					if wcupd.Data.IsEmpty() {
						DoLog(bot, cid,
							fmt.Sprintf(client.GetLocale().ErrorDetected,
								fmt.Sprintf(client.GetLocale().NoSuchMediaRID, wcupd.Data.GetId())))
					} else {
						media := tgbotapi.NewPhoto(cid, wcupd.Data)
						media.Caption = fmt.Sprintf(client.GetLocale().MediaRID, wcupd.Data.GetId())
						bot.Send(media)
					}
				}
			}
		}
	}()

	go func() {
		//initialization
		SendInitCommands(bot, TgUserId{0, 0}, DefaultLocale())

		paramCheckRegexp, _ := regexp.Compile("^[a-zA-Z_]+[a-zA-Z_0-9]*$")

		for update := range updates {
			error_str := ""
			var error_chat_id int64 = 0
			if update.CallbackQuery != nil { // If we got a callbask
				tgid := TgUserId{
					user_id: update.CallbackQuery.From.ID,
					chat_id: update.CallbackQuery.Message.Chat.ID,
				}
				client := clientpool.ByUCID(tgid)
				error_chat_id = tgid.chat_id

				if client != nil {
					if !client.IsAuthorized() {
						msg := SendToAuthorize(tgid.chat_id, client.GetUserName(), client.GetLocale())
						bot.Send(msg)
					}

					comm, params := ParseCommand(update.CallbackQuery.Data)
					switch comm {
					case TG_COMMAND_GETRID:
						{
							GetRid(clientpool, client, params)
						}
					case TG_COMMAND_TARG:
						{
							if len(params) > 0 {
								msg := SetTarget(clientpool, client, params[0])
								bot.Send(msg)
							} else {
								error_str = client.locale.EmptyTarget
							}
						}
					case TG_COMMAND_TOJSON:
						{
							CollapseParams(params)
							if len(params) > 0 {
								wcmsg, err := DecodeJSONMessage(params[0])
								if err == nil {
									txt, mkp, err := RebuildMessageEditor(wcmsg, client)
									if err == nil {
										msg_edit := tgbotapi.NewMessage(
											client.GetChatID(),
											FormatOutputMessage(txt))
										msg_edit.ParseMode = PM_HTML
										msg_edit.ReplyMarkup = mkp
										bot.Send(msg_edit)
									} else {
										error_str = ErrorToString(err)
									}
								} else {
									error_str = ErrorToString(err)
								}
							}
						}
					case TG_COMMAND_SEND, TG_COMMAND_ADD_PARAM, TG_COMMAND_EDIT_PARAM, TG_COMMAND_DEL_PARAM:
						{
							msg := update.CallbackQuery.Message
							if len(msg.Text) > 0 {
								wcmsg, err := DecodeJSONMessage(msg.Text)
								if err == nil {
									cnt := 0
									if wcmsg.Params != nil {
										cnt = len(wcmsg.Params)
									}

									switch comm {
									case TG_COMMAND_SEND:
										{
											if strings.Compare(client.GetTarget(), ALL_DEVICES) != 0 {
												wcmsg.Target = client.GetTarget()
											}
											client.GetWCClient().SendMsgs(wcmsg)
										}
									case TG_COMMAND_ADD_PARAM:
										{
											inline_msg, _ := GenJSONMessageInline(wcmsg)

											msgr := tgbotapi.NewMessage(client.GetChatID(),
												fmt.Sprintf(
													"%s %s\n"+
														"<pre>%s</pre>",
													client.GetLocale().SetNewParamName,
													TG_COMMAND_ADD_PARAM,
													inline_msg))
											msgr.ParseMode = PM_HTML
											msgr.ReplyMarkup = tgbotapi.ForceReply{
												ForceReply:            true,
												InputFieldPlaceholder: fmt.Sprintf("par%d", cnt),
											}
											bot.Send(msgr)
										}
									case TG_COMMAND_EDIT_PARAM:
										{
											if len(params) > 0 {
												inline_msg, _ := GenJSONMessageInline(wcmsg)

												msgr := tgbotapi.NewMessage(client.GetChatID(),
													fmt.Sprintf(
														"%s %s_%s\n"+
															"<pre>%s</pre>",
														client.GetLocale().SetNewParamValue,
														TG_COMMAND_EDIT_PARAM,
														params[0],
														inline_msg))
												msgr.ParseMode = PM_HTML
												msgr.ReplyMarkup = tgbotapi.ForceReply{
													ForceReply:            true,
													InputFieldPlaceholder: fmt.Sprintf("%v", wcmsg.Params[params[0]]),
												}
												bot.Send(msgr)
											} else {
												error_str = client.GetLocale().NoParams
											}
										}
									case TG_COMMAND_DEL_PARAM:
										{
											if wcmsg.Params != nil && len(params) > 0 {
												delete(wcmsg.Params, params[0])

												txt, mkp, err := RebuildMessageEditor(wcmsg, client)
												if err == nil {
													msg_edit := tgbotapi.NewEditMessageTextAndMarkup(client.GetChatID(),
														update.CallbackQuery.Message.MessageID,
														FormatOutputMessage(txt),
														mkp)
													msg_edit.ParseMode = PM_HTML
													bot.Send(msg_edit)
												} else {
													error_str = ErrorToString(err)
												}
											}
										}
									}
								} else {
									error_str = ErrorToString(err)
								}
							} else {
								error_str = client.GetLocale().EmptyCallback
							}
						}
					}

				}
				req := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
				bot.Send(req)
			}
			if update.Message != nil { // If we got a message
				log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

				tgid := TgUserId{user_id: update.Message.From.ID, chat_id: update.Message.Chat.ID}
				client := clientpool.ByUCID(tgid)

				if client == nil {
					client, err = clientpool.AddCID(tgid,
						update.Message.From.UserName,
						update.Message.From.FirstName,
						update.Message.From.LastName,
						GetLocale(update.Message.From.LanguageCode))
					check(err)
					SendInitCommands(bot, tgid, client.GetLocale())
				}
				error_chat_id = tgid.chat_id

				comm, params := ParseCommand(update.Message.Text)

				switch comm {
				case TG_COMMAND_START:
					{
						if client.IsAuthorized() {
							msg := SendAuthorized(update.Message.Chat.ID, update.Message.From.UserName, client.GetLocale())
							bot.Send(msg)
						} else {
							// if not authorized
							if client.CanAutoAuthorize() {
								clientpool.Authorize(client)
							} else {
								msg := tgbotapi.NewMessage(update.Message.Chat.ID,
									fmt.Sprintf(client.GetLocale().Greetings,
										TG_COMMAND_AUTH, TG_COMMAND_AUTH))
								msg.ParseMode = PM_HTML
								bot.Send(msg)
							}
						}
					}
				case TG_COMMAND_AUTH:
					{
						if client.IsAuthorized() {
							msg := SendAuthorized(update.Message.Chat.ID, update.Message.From.UserName, client.GetLocale())
							bot.Send(msg)
						} else {
							client.SetStatus(StatusWaitLogin)

							msg := tgbotapi.NewMessage(update.Message.Chat.ID, client.GetLocale().LoginPrompt)
							msg.ParseMode = PM_HTML
							bot.Send(msg)
						}
					}
				case TG_COMMAND_LOGOUT:
					{
						if !client.IsAuthorized() {
							msg := SendToAuthorize(update.Message.Chat.ID, update.Message.From.UserName, client.GetLocale())
							bot.Send(msg)
						} else {
							client.GetWCClient().Disconnect()
						}
					}
				case TG_COMMAND_TARG:
					{
						if !client.IsAuthorized() {
							msg := SendToAuthorize(update.Message.Chat.ID, update.Message.From.UserName, client.GetLocale())
							bot.Send(msg)
						} else {
							client.SetStatus(StatusWaitSetTarget)

							page := fmt.Sprintf(client.GetLocale().TargetPrompt,
								TG_COMMAND_DEVS, TG_COMMAND_DEVS,
								client.GetTarget())

							msg := tgbotapi.NewMessage(update.Message.Chat.ID, page)
							msg.ParseMode = PM_HTML
							bot.Send(msg)
						}
					}
				case TG_COMMAND_FILTER:
					{
						if !client.IsAuthorized() {
							msg := SendToAuthorize(update.Message.Chat.ID, update.Message.From.UserName, client.GetLocale())
							bot.Send(msg)
						} else {
							page := fmt.Sprintf(client.GetLocale().FilterPrompt,
								client.GetFilter())

							client.SetStatus(StatusWaitSetFilter)
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, page)
							msg.ParseMode = PM_HTML
							bot.Send(msg)
						}
					}
				case TG_COMMAND_DEVS:
					{
						if !client.IsAuthorized() {
							msg := SendToAuthorize(update.Message.Chat.ID, update.Message.From.UserName, client.GetLocale())
							bot.Send(msg)
						} else {
							clientpool.UpdateDevices(client)
						}
					}
				case TG_COMMAND_GETRID:
					{
						GetRid(clientpool, client, params)
					}
				default:
					{
						switch client.GetStatus() {
						case StatusWaitLogin:
							{
								client.SetLogin(update.Message.Text)
								client.SetStatus(StatusWaitPassword)
								msg := tgbotapi.NewMessage(update.Message.Chat.ID, client.GetLocale().PasswordPrompt)
								msg.ParseMode = PM_HTML
								bot.Send(msg)
							}
						case StatusWaitPassword:
							{
								client.SetPwd(update.Message.Text)
								// delete user's password message
								req := tgbotapi.NewDeleteMessage(update.Message.Chat.ID, update.Message.MessageID)
								bot.Send(req)
								// authorize
								clientpool.Authorize(client)
							}
						case StatusWaitSetTarget:
							{
								client.SetStatus(StatusAuthorized)
								msg := SetTarget(clientpool, client, update.Message.Text)
								bot.Send(msg)
							}
						case StatusWaitSetFilter:
							{
								err = clientpool.SetClientFilter(client, update.Message.Text)
								check(err)
								client.SetStatus(StatusAuthorized)
								msg := tgbotapi.NewMessage(update.Message.Chat.ID,
									fmt.Sprintf(client.GetLocale().FilterUpdated, update.Message.Text))
								bot.Send(msg)
							}
						default:
							{
								// check if reply
								if update.Message.ReplyToMessage != nil {
									if err == nil && (len(update.Message.ReplyToMessage.Entities) > 1) {
										// extract json obj
										cur_cmd := ""
										cur_obj := ""
										for _, ent := range update.Message.ReplyToMessage.Entities {
											switch ent.Type {
											case "bot_command":
												{
													str := string([]rune(update.Message.ReplyToMessage.Text)[ent.Offset : ent.Offset+ent.Length])
													cur_cmd, params = ParseCommand(str)
													CollapseParams(params)
												}
											case "pre":
												{
													cur_obj = string([]rune(update.Message.ReplyToMessage.Text)[ent.Offset : ent.Offset+ent.Length])
												}
											}
										}

										if (len(cur_cmd) > 0) && (len(cur_obj) > 0) {
											obj, err := DecodeJSONMessage(cur_obj)

											switch cur_cmd {
											case TG_COMMAND_ADD_PARAM:
												{
													param_v := strings.TrimSpace(update.Message.Text)
													// check params
													regres := paramCheckRegexp.MatchString(param_v)

													if regres {
														if obj.Params == nil {
															obj.Params = make(map[string]any)
														}
														obj.Params[update.Message.Text] = 0
													} else {
														msg_new := tgbotapi.NewMessage(update.Message.Chat.ID,
															fmt.Sprintf(client.GetLocale().WrongParamName, param_v))
														msg_new.ParseMode = PM_HTML
														bot.Send(msg_new)
													}
												}
											case TG_COMMAND_EDIT_PARAM:
												{
													param_v := strings.TrimSpace(update.Message.Text)

													if obj.Params == nil {
														obj.Params = make(map[string]any)
													}
													// check is boolean
													boolv, err := strconv.ParseBool(strings.ToLower(param_v))
													if err == nil {
														obj.Params[params[0]] = boolv
														break
													}
													// check is int
													intv, err := strconv.ParseInt(strings.ToLower(param_v), 10, 64)
													if err == nil {
														obj.Params[params[0]] = intv
														break
													}
													// check is float
													floatv, err := strconv.ParseFloat(strings.ToLower(param_v), 64)
													if err == nil {
														obj.Params[params[0]] = floatv
														break
													}
													obj.Params[params[0]] = param_v
													err = nil
												}
											}
											if err == nil {
												txt, mkp, err := RebuildMessageEditor(obj, client)
												if err == nil {
													msg_new := tgbotapi.NewMessage(update.Message.Chat.ID,
														FormatOutputMessage(txt))
													msg_new.ParseMode = PM_HTML
													msg_new.ReplyMarkup = mkp
													bot.Send(msg_new)
												} else {
													error_str = ErrorToString(err)
												}
											}
										}
									}
								} else {
									if update.Message.Photo != nil {
										// try to get the new media and send it to host
										// find the best media size
										bst := update.Message.Photo[0]
										for _, mipmap := range update.Message.Photo {
											if mipmap.FileSize < wcb_cfg.MaxMediaSize { // check for limit
												bst = mipmap
											}
										}

										file, err := bot.GetFile(tgbotapi.FileConfig{FileID: bst.FileID})
										if err == nil {
											go func(cl *PoolClient, url tgbotapi.File) {
												ext := strings.ToUpper(path.Ext(url.FilePath))
												if len(ext) > 0 {
													ext = ext[1:]
												}

												// Get the data
												resp, err := http.Get(
													fmt.Sprintf(tgbotapi.FileEndpoint,
														wcb_cfg.BotToken,
														url.FilePath))
												if err != nil {
													return
												}

												// Check server response
												if resp.StatusCode != http.StatusOK {
													resp.Body.Close()
													return
												}

												cl.GetWCClient().SaveRecord(resp.Body, resp.ContentLength, ext, nil)
											}(client, file)
										} else {
											error_str = ErrorToString(err)
										}
									} else if update.Message.Text != "" {
										txt, mkp, err := RebuildMessageEditor(&wc.OutMessageStruct{Msg: update.Message.Text}, client)
										if err == nil {
											msg := tgbotapi.NewMessage(update.Message.Chat.ID,
												FormatOutputMessage(txt))
											msg.ParseMode = PM_HTML
											msg.ReplyMarkup = mkp
											msg.ReplyToMessageID = update.Message.MessageID
											bot.Send(msg)
										} else {
											error_str = ErrorToString(err)
										}
									} else {
										error_str = client.GetLocale().UnsupportedMsg
									}
								}
							}
						}
					}
				}
			}
			if len(error_str) > 0 && (error_chat_id > 0) {
				DoLog(bot, error_chat_id,
					fmt.Sprintf(DefaultLocale().ErrorDetected, error_str))
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
