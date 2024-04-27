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

var ParamCheckRegexp *regexp.Regexp

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

const JSON_FILTER = "filter"

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
const TG_COMMAND_MSG_TOJSON = "/msgtojson"

/* Global methods */

func FormatOutputMessage(txt string) string {
	return fmt.Sprintf("<pre><code class=\"language-json\">%s</code></pre>", txt)
}

func FormatFileUrl(bottoken string, path string) string {
	return fmt.Sprintf(tgbotapi.FileEndpoint,
		bottoken,
		path)
}

func SendMediaToHost(cl *PoolClient, fullurl string) {
	ext := strings.ToUpper(path.Ext(fullurl))
	if len(ext) > 0 {
		ext = ext[1:]
	}
	// Get the data
	resp, err := http.Get(fullurl)
	if err != nil {
		return
	}
	// Check server response
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return
	}
	cl.GetWCClient().SaveRecord(resp.Body, resp.ContentLength, ext, nil)
}

func PrepareInitCommands(tid TgUserId, locale *LanguageStrings) tgbotapi.Chattable {
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

	return req
}

func PrepareWorkingCommands(client *PoolClient) tgbotapi.Chattable {
	locale := client.GetLocale()
	req := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: TG_COMMAND_LOGOUT, Description: locale.CommandLogOut},
		tgbotapi.BotCommand{Command: TG_COMMAND_SETTS, Description: locale.CommandSett},
		tgbotapi.BotCommand{Command: TG_COMMAND_DEVS, Description: locale.CommandDevs},
	)
	req.Scope = &tgbotapi.BotCommandScope{Type: "chat", ChatID: client.GetChatID()} //, UserID: tid.user_id}

	return req
}

func PrepareDoLog(chatid int64, value string) tgbotapi.MessageConfig {
	msg := tgbotapi.NewMessage(chatid, value)
	msg.ParseMode = PM_HTML

	return msg
}

func PrepareAuthorized(client *PoolClient) tgbotapi.MessageConfig {
	// if already authorized
	msg := tgbotapi.NewMessage(client.GetChatID(),
		fmt.Sprintf(client.GetLocale().AlreadyAuthorized, client.GetUserName()))
	msg.ParseMode = PM_HTML
	return msg
}

func PrepareToAuthorize(client *PoolClient) tgbotapi.MessageConfig {
	msg := tgbotapi.NewMessage(client.GetChatID(),
		fmt.Sprintf(client.GetLocale().NotAuthorized, client.GetUserName(), TG_COMMAND_AUTH, TG_COMMAND_AUTH))
	msg.ParseMode = PM_HTML
	return msg
}

func PrepareSetTarget(clientpool *Pool, client *PoolClient, trg string) tgbotapi.MessageConfig {
	err := clientpool.SetClientTarget(client, trg)
	check(err)
	msg := tgbotapi.NewMessage(client.GetChatID(),
		fmt.Sprintf(client.GetLocale().TargetUpdated, trg))
	return msg
}

func PrepareSetFilter(clientpool *Pool, client *PoolClient, trg string) tgbotapi.MessageConfig {
	err := clientpool.SetClientFilter(client, trg)
	check(err)
	msg := tgbotapi.NewMessage(client.GetChatID(),
		fmt.Sprintf(client.GetLocale().FilterUpdated, trg))
	return msg
}

func RebuildMessageEditor(msg *wc.OutMessageStruct, client *PoolClient) (string, tgbotapi.InlineKeyboardMarkup, error) {
	txt, err := GenJSONMessageText(msg)

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

func RebuildSettingsEditor(client *PoolClient) (string, tgbotapi.InlineKeyboardMarkup, error) {
	txt, err := GenJSONSettingsText(client.GetSettings())

	if err != nil {
		return "", tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{}), err
	}

	var buttons [][]tgbotapi.InlineKeyboardButton

	buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(client.GetLocale().CommandTarget, TG_COMMAND_TARG),
	))
	buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(client.GetLocale().CommandFilter, TG_COMMAND_FILTER),
	))

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

func GenJSONSettingsMap(sett *PoolClientSettings) map[string]any {
	sett_map := make(map[string]any)
	sett_map[wc.JSON_RPC_TARGET] = sett.Target
	sett_map[JSON_FILTER] = sett.Filter
	return sett_map
}

func GenJSONSettingsText(sett *PoolClientSettings) (string, error) {
	sett_map := GenJSONSettingsMap(sett)
	b, err := json.MarshalIndent(sett_map, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func GenJSONSettingsTextInline(sett *PoolClientSettings) (string, error) {
	sett_map := GenJSONSettingsMap(sett)
	b, err := json.Marshal(sett_map)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func DecodeJSONSettingsText(msg string) (*PoolClientSettings, error) {
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
		{name: JSON_FILTER, tp: reflect.String},
	}

	st := &PoolClientSettings{}

	for num, v := range decl {
		_any, ok := jsonmap[v.name]
		if ok {
			dt := reflect.TypeOf(_any).Kind()
			if dt == v.tp {
				switch num {
				case 0:
					st.Target, _ = _any.(string)
				case 1:
					st.Filter, _ = _any.(string)
				}
			} else {
				return nil, wc.ThrowErrMalformedResponse(wc.EMKWrongType, v.name, fmt.Sprintf("%v", v.tp))
			}
		} else {
			return nil, fmt.Errorf("%s param is not provided", v.name)
		}
	}

	return st, nil
}

func GenJSONMessageMap(msg *wc.OutMessageStruct) map[string]any {
	msg_map := make(map[string]any)
	msg_map[wc.JSON_RPC_MSG] = msg.Msg
	if msg.Params != nil && len(msg.Params) > 0 {
		msg_map[wc.JSON_RPC_PARAMS] = msg.Params
	}
	return msg_map
}

func GenJSONMessageText(msg *wc.OutMessageStruct) (string, error) {
	msg_map := GenJSONMessageMap(msg)
	b, err := json.MarshalIndent(msg_map, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func GenJSONMessageTextInline(msg *wc.OutMessageStruct) (string, error) {
	msg_map := GenJSONMessageMap(msg)
	b, err := json.Marshal(msg_map)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func DecodeJSONMessageText(msg string) (*wc.OutMessageStruct, error) {
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

	if len(mr.Msg) == 0 {
		// not a message object
		return nil, wc.ThrowErrWrongOutMsgFormat()
	}

	return mr, nil
}

func ExtractArgsFromTgUpdate(clientpool *Pool, update *tgbotapi.Update) (TgUserId, *PoolClient) {
	var tgid TgUserId

	if update.CallbackQuery != nil { // If we got a callback
		tgid.user_id = update.CallbackQuery.From.ID
		tgid.chat_id = update.CallbackQuery.Message.Chat.ID
	} else if update.Message != nil { // If we got a message
		tgid.user_id = update.Message.From.ID
		tgid.chat_id = update.Message.Chat.ID
	}
	client := clientpool.ByUCID(tgid)

	return tgid, client
}

func ExtractJsonObjFromEntities(msg string, entities []tgbotapi.MessageEntity) (string, []string, string, bool) {
	cur_cmd := ""
	cur_obj := ""
	var params []string = make([]string, 0)
	for _, ent := range entities {
		switch ent.Type {
		case "bot_command":
			{
				str := string([]rune(msg)[ent.Offset : ent.Offset+ent.Length])
				cur_cmd, params = ParseCommand(str)
				CollapseParams(params)
			}
		case "pre":
			{
				cur_obj = string([]rune(msg)[ent.Offset : ent.Offset+ent.Length])
			}
		}
	}
	return cur_cmd, params, cur_obj, (len(cur_cmd) > 0) && (len(cur_obj) > 0)
}

/* Bot handler */

type BotHandler struct {
	Bot        *tgbotapi.BotAPI
	ChatId     int64
	Client     *PoolClient // may be nill
	ClientPool *Pool       // may be nill
	Command    *string     // may be nill
	Params     []string    // may be nill
	ErrorStr   string
}

func NewHandler(bot *tgbotapi.BotAPI, client *PoolClient) *BotHandler {
	return &BotHandler{Bot: bot, Client: client}
}

func NewCommandHandler(bot *tgbotapi.BotAPI, client *PoolClient, clientpool *Pool, command *string, params []string) *BotHandler {
	return &BotHandler{Bot: bot, Client: client, ClientPool: clientpool, Command: command, Params: params}
}

func (handler *BotHandler) GetLocale() *LanguageStrings {
	if handler.Client != nil {
		return handler.Client.GetLocale()
	} else {
		return DefaultLocale()
	}
}

func (handler *BotHandler) Send(msg tgbotapi.Chattable) {
	if handler.Bot != nil {
		handler.Bot.Send(msg)
	}
}

func (handler *BotHandler) GetUserName() string {
	if handler.Client != nil {
		return handler.Client.GetUserName()
	} else {
		return ""
	}
}

func (handler *BotHandler) GetChatID() int64 {
	if handler.Client != nil {
		return handler.Client.GetChatID()
	} else {
		return handler.ChatId
	}
}

func (handler *BotHandler) HandleWCMessage(wcupd *WCUpdate) {
	var table string
	headers, rows := MapToTable(wcupd.Msg.Params)
	if headers != nil {
		table = FormatTable(headers, rows)
	}

	json_text, _ := GenJSONMessageTextInline(
		&wc.OutMessageStruct{
			Msg:    wcupd.Msg.Msg,
			Params: wcupd.Msg.Params,
		})

	msg := tgbotapi.NewMessage(handler.GetChatID(),
		fmt.Sprintf("<pre>%s</pre>\n%s\n<pre>%s</pre>\n<pre>%s</pre>",
			wcupd.Msg.Device, handler.GetLocale().IncomingMsg,
			wcupd.Msg.Msg, table))
	msg.ParseMode = PM_HTML
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			handler.GetLocale().ToJSON,
			TG_COMMAND_MSG_TOJSON+"_"+json_text),
	})
	handler.Send(msg)
}

func (handler *BotHandler) HandleWCMedia(wcupd *WCUpdate) {
	if strings.Compare(wcupd.Rec.Device, handler.GetUserName()) != 0 {
		msg := tgbotapi.NewMessage(handler.GetChatID(),
			fmt.Sprintf("<pre>%s</pre>\n%s\n<pre>%s</pre>\n"+
				"<a href=\"%s_%d\">%s_%d</a>",
				wcupd.Rec.Device, handler.GetLocale().IncomingMedia,
				wcupd.Rec.Stamp,
				TG_COMMAND_GETRID, int64(wcupd.Rec.Rid),
				TG_COMMAND_GETRID, int64(wcupd.Rec.Rid)))
		msg.ParseMode = PM_HTML
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(handler.GetLocale().GetMedia,
				fmt.Sprintf("%s_%d", TG_COMMAND_GETRID, int64(wcupd.Rec.Rid))),
		})
		handler.Send(msg)
	}
}

func (handler *BotHandler) HandleWCMediaData(wcupd *WCUpdate) {
	if wcupd.Data.IsEmpty() {
		handler.Send(PrepareDoLog(handler.GetChatID(),
			fmt.Sprintf(handler.GetLocale().ErrorDetected,
				fmt.Sprintf(handler.GetLocale().NoSuchMediaRID, wcupd.Data.GetId()))))
	} else {
		media := tgbotapi.NewPhoto(handler.GetChatID(), wcupd.Data)
		media.Caption = fmt.Sprintf(handler.GetLocale().MediaRID, wcupd.Data.GetId())
		handler.Send(media)
	}
}

func (handler *BotHandler) HandleGetRid() {
	GetRid(handler.ClientPool, handler.Client, handler.Params)
}

func (handler *BotHandler) SendSettingsAsJson() {
	txt, mkp, err := RebuildSettingsEditor(handler.Client)
	if err == nil {
		msg_new := tgbotapi.NewMessage(
			handler.GetChatID(),
			FormatOutputMessage(txt))
		msg_new.ParseMode = PM_HTML
		msg_new.ReplyMarkup = mkp
		handler.Send(msg_new)
	} else {
		handler.ErrorStr = ErrorToString(err)
	}
}

func (handler *BotHandler) HandleSetTarget() {
	if len(handler.Params) > 0 {
		handler.Send(PrepareSetTarget(handler.ClientPool, handler.Client, handler.Params[0]))
	} else {
		handler.HandleSettingsSetTarget(handler.Client.GetSettings())
	}
}

func (handler *BotHandler) HandleSettingsSetTarget(sett *PoolClientSettings) {
	inline_msg, _ := GenJSONSettingsTextInline(sett)

	page := fmt.Sprintf(handler.GetLocale().TargetPrompt,
		TG_COMMAND_DEVS, TG_COMMAND_DEVS,
		handler.Client.GetTarget())
	msgr := tgbotapi.NewMessage(handler.GetChatID(),
		fmt.Sprintf(
			"%s %s\n<pre>%s</pre>",
			page,
			TG_COMMAND_TARG,
			inline_msg))
	msgr.ParseMode = PM_HTML
	msgr.ReplyMarkup = tgbotapi.ForceReply{
		ForceReply:            true,
		InputFieldPlaceholder: sett.Target,
	}
	handler.Send(msgr)
}

func (handler *BotHandler) HandleSettingsSetFilter(sett *PoolClientSettings) {
	inline_msg, _ := GenJSONSettingsTextInline(sett)

	page := fmt.Sprintf(handler.GetLocale().FilterPrompt,
		handler.Client.GetFilter())
	msgr := tgbotapi.NewMessage(handler.GetChatID(),
		fmt.Sprintf(
			"%s %s\n<pre>%s</pre>",
			page,
			TG_COMMAND_FILTER,
			inline_msg))
	msgr.ParseMode = PM_HTML
	msgr.ReplyMarkup = tgbotapi.ForceReply{
		ForceReply:            true,
		InputFieldPlaceholder: sett.Filter,
	}
	handler.Send(msgr)
}

func (handler *BotHandler) SendMessageAsJson(wcmsg *wc.OutMessageStruct, replyto int) {
	txt, mkp, err := RebuildMessageEditor(wcmsg, handler.Client)
	if err == nil {
		msg_new := tgbotapi.NewMessage(
			handler.GetChatID(),
			FormatOutputMessage(txt))
		msg_new.ParseMode = PM_HTML
		msg_new.ReplyMarkup = mkp
		if replyto > 0 {
			msg_new.ReplyToMessageID = replyto
		}
		handler.Send(msg_new)
	} else {
		handler.ErrorStr = ErrorToString(err)
	}
}

func (handler *BotHandler) HandleMessageToJson() {
	CollapseParams(handler.Params)
	if len(handler.Params) > 0 {
		wcmsg, err := DecodeJSONMessageText(handler.Params[0])
		if err == nil {
			handler.SendMessageAsJson(wcmsg, -1)
		} else {
			handler.ErrorStr = ErrorToString(err)
		}
	}
}

func (handler *BotHandler) HandleOutMessageSend(wcmsg *wc.OutMessageStruct) {
	if strings.Compare(handler.Client.GetTarget(), ALL_DEVICES) != 0 {
		wcmsg.Target = handler.Client.GetTarget()
	}
	handler.Client.GetWCClient().SendMsgs(wcmsg)
}

func (handler *BotHandler) HandleOutMessageAddParam(wcmsg *wc.OutMessageStruct) {
	inline_msg, _ := GenJSONMessageTextInline(wcmsg)

	cnt := 0
	if wcmsg.Params != nil {
		cnt = len(wcmsg.Params)
	}

	msgr := tgbotapi.NewMessage(handler.GetChatID(),
		fmt.Sprintf(
			"%s %s\n<pre>%s</pre>",
			handler.GetLocale().SetNewParamName,
			TG_COMMAND_ADD_PARAM,
			inline_msg))
	msgr.ParseMode = PM_HTML
	msgr.ReplyMarkup = tgbotapi.ForceReply{
		ForceReply:            true,
		InputFieldPlaceholder: fmt.Sprintf("par%d", cnt),
	}
	handler.Send(msgr)
}

func (handler *BotHandler) HandleOutMessageDoAddParam(wcmsg *wc.OutMessageStruct, value string) {
	param_v := strings.TrimSpace(value)
	// check params
	regres := ParamCheckRegexp.MatchString(param_v)

	if regres {
		if wcmsg.Params == nil {
			wcmsg.Params = make(map[string]any)
		}
		wcmsg.Params[param_v] = 0
	} else {
		msg_new := tgbotapi.NewMessage(handler.GetChatID(),
			fmt.Sprintf(handler.GetLocale().WrongParamName, param_v))
		msg_new.ParseMode = PM_HTML
		handler.Send(msg_new)
	}
}

func (handler *BotHandler) HandleOutMessageEditParam(wcmsg *wc.OutMessageStruct) {
	if len(handler.Params) > 0 {
		inline_msg, _ := GenJSONMessageTextInline(wcmsg)

		msgr := tgbotapi.NewMessage(handler.GetChatID(),
			fmt.Sprintf(
				"%s %s_%s\n<pre>%s</pre>",
				handler.GetLocale().SetNewParamValue,
				TG_COMMAND_EDIT_PARAM,
				handler.Params[0],
				inline_msg))
		msgr.ParseMode = PM_HTML
		msgr.ReplyMarkup = tgbotapi.ForceReply{
			ForceReply:            true,
			InputFieldPlaceholder: fmt.Sprintf("%v", wcmsg.Params[handler.Params[0]]),
		}
		handler.Send(msgr)
	} else {
		handler.ErrorStr = handler.GetLocale().NoParams
	}
}

func (handler *BotHandler) HandleOutMessageDoEditParam(wcmsg *wc.OutMessageStruct, value string) {
	param_v := strings.TrimSpace(value)

	if wcmsg.Params == nil {
		wcmsg.Params = make(map[string]any)
	}
	// check is boolean
	boolv, err := strconv.ParseBool(strings.ToLower(param_v))
	if err == nil {
		wcmsg.Params[handler.Params[0]] = boolv
		return
	}
	// check is int
	intv, err := strconv.ParseInt(strings.ToLower(param_v), 10, 64)
	if err == nil {
		wcmsg.Params[handler.Params[0]] = intv
		return
	}
	// check is float
	floatv, err := strconv.ParseFloat(strings.ToLower(param_v), 64)
	if err == nil {
		wcmsg.Params[handler.Params[0]] = floatv
		return
	}
	wcmsg.Params[handler.Params[0]] = param_v
}

func (handler *BotHandler) HandleOutMessageDeleteParam(wcmsg *wc.OutMessageStruct, mid int) {
	if wcmsg.Params != nil && len(handler.Params) > 0 {
		delete(wcmsg.Params, handler.Params[0])

		txt, mkp, err := RebuildMessageEditor(wcmsg, handler.Client)
		if err == nil {
			msg_edit := tgbotapi.NewEditMessageTextAndMarkup(handler.GetChatID(),
				mid,
				FormatOutputMessage(txt),
				mkp)
			msg_edit.ParseMode = PM_HTML
			handler.Send(msg_edit)
		} else {
			handler.ErrorStr = ErrorToString(err)
		}
	}
}

func (handler *BotHandler) HandleEditOutMessage(ref string, mid int) {
	if len(ref) > 0 {
		wcmsg, err := DecodeJSONMessageText(ref)
		if err == nil {
			switch *handler.Command {
			case TG_COMMAND_SEND:
				{
					handler.HandleOutMessageSend(wcmsg)
				}
			case TG_COMMAND_ADD_PARAM:
				{
					handler.HandleOutMessageAddParam(wcmsg)
				}
			case TG_COMMAND_EDIT_PARAM:
				{
					handler.HandleOutMessageEditParam(wcmsg)
				}
			case TG_COMMAND_DEL_PARAM:
				{
					handler.HandleOutMessageDeleteParam(wcmsg, mid)
				}
			}
		} else {
			handler.ErrorStr = ErrorToString(err)
		}
	} else {
		handler.ErrorStr = handler.GetLocale().EmptyCallback
	}
}

func (handler *BotHandler) HandleStart() {
	if handler.Client.IsAuthorized() {
		handler.Send(PrepareAuthorized(handler.Client))
	} else {
		// if not authorized
		if handler.Client.CanAutoAuthorize() {
			handler.ClientPool.Authorize(handler.Client)
		} else {
			msg := tgbotapi.NewMessage(handler.GetChatID(),
				fmt.Sprintf(handler.GetLocale().Greetings,
					TG_COMMAND_AUTH, TG_COMMAND_AUTH))
			msg.ParseMode = PM_HTML
			handler.Send(msg)
		}
	}
}

func (handler *BotHandler) HandleAuth() {
	if handler.Client.IsAuthorized() {
		handler.Send(PrepareAuthorized(handler.Client))
	} else {
		handler.Client.SetStatus(StatusWaitLogin)

		msg := tgbotapi.NewMessage(handler.GetChatID(), handler.GetLocale().LoginPrompt)
		msg.ParseMode = PM_HTML
		handler.Send(msg)
	}
}

func (handler *BotHandler) HandleLogOut() {
	if !handler.Client.IsAuthorized() {
		handler.Send(PrepareToAuthorize(handler.Client))
	} else {
		handler.Client.GetWCClient().Disconnect()
	}
}

func (handler *BotHandler) HandleDevices() {
	if !handler.Client.IsAuthorized() {
		handler.Send(PrepareToAuthorize(handler.Client))
	} else {
		handler.ClientPool.UpdateDevices(handler.Client)
	}
}

/* WCClient listener */

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
		a.bot.Send(PrepareWorkingCommands(client))
	}
}

func (a Listener) OnConnected(client *PoolClient, status wc.ClientStatus) {
	if client != nil {
		page := fmt.Sprintf(client.GetLocale().ClientStatusChanged, wc.ClientStatusText(status))
		msg := tgbotapi.NewMessage(client.GetChatID(), page)
		msg.ParseMode = PM_HTML

		a.bot.Send(msg)

		if status == wc.StateDisconnected {
			a.bot.Send(PrepareInitCommands(client.GetID(), client.GetLocale()))
		}
	}
}

func (a Listener) OnAddLog(client *PoolClient, value string) {
	if client != nil {
		a.bot.Send(PrepareDoLog(client.GetChatID(), value))
	}
}

func (a Listener) OnUpdateDevices(client *PoolClient, devices []map[string]any) {
	if client != nil {
		headers, rows := MapArrayToTable(devices)
		buttons := make([][]tgbotapi.InlineKeyboardButton, 0, len(devices))

		for _, dev := range devices {
			name := fmt.Sprintf("%v", dev[wc.JSON_RPC_DEVICE])
			if strings.Compare(name, client.GetUserName()) != 0 {
				btn := tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(
						fmt.Sprintf(client.GetLocale().SetTarget, name),
						fmt.Sprintf("%s&%s", TG_COMMAND_TARG, name)))
				buttons = append(buttons, btn)
			}
		}
		btn := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				client.GetLocale().SetTargetAll,
				fmt.Sprintf("%s&%s", TG_COMMAND_TARG, ALL_DEVICES)))
		buttons = append(buttons, btn)

		if headers != nil {
			table := FormatTable(headers, rows)

			msg := tgbotapi.NewMessage(client.GetChatID(), "<pre>"+table+"</pre>")
			msg.ParseMode = PM_HTML
			msg.ReplyMarkup = tgbotapi.InlineKeyboardMarkup{InlineKeyboard: buttons}

			a.bot.Send(msg)
		}
	}
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

	// prepare regular expressions
	ParamCheckRegexp, _ = regexp.Compile("^[a-zA-Z_]+[a-zA-Z_0-9]*$")

	// prepare chans
	stop := make(chan int)
	updates := bot.GetUpdatesChan(u)
	pooltimer := clientpool.GetPoolTimer()

	// start WC handler
	go func() {
		for wcupd := range pooltimer {
			handler := NewHandler(bot, wcupd.Client) // gen new handler
			switch wcupd.Type {
			case Message:
				{
					// we'v got a message from some device - proceed
					handler.HandleWCMessage(&wcupd)
				}
			case Media:
				{
					// we'v got a media notification from some device - proceed
					handler.HandleWCMedia(&wcupd)
				}
			case MediaData:
				{
					// we'v got a media data from some device - proceed
					handler.HandleWCMediaData(&wcupd)
				}
			}
		}
	}()

	// start TG handler
	go func() {
		//initialization
		bot.Send(PrepareInitCommands(TgUserId{0, 0}, DefaultLocale()))

		for update := range updates {
			// try to extract ids and find the corresponding client object
			tgid, client := ExtractArgsFromTgUpdate(clientpool, &update)
			// declare the error holder variable
			error_str := ""

			if update.CallbackQuery != nil { // If we got a callback
				if client != nil {
					if !client.IsAuthorized() {
						// ignore callbacks from non-authorized clients
						bot.Send(PrepareToAuthorize(client))
					} else {
						comm, params := ParseCommand(update.CallbackQuery.Data)
						handler := NewCommandHandler(bot, client, clientpool, &comm, params)

						switch comm {
						case TG_COMMAND_GETRID:
							{
								handler.HandleGetRid()
							}
						case TG_COMMAND_TARG:
							{
								handler.HandleSetTarget()
							}
						case TG_COMMAND_FILTER:
							{
								handler.HandleSettingsSetFilter(client.GetSettings())
							}
						case TG_COMMAND_MSG_TOJSON:
							{
								handler.HandleMessageToJson()
							}
						case TG_COMMAND_SEND, TG_COMMAND_ADD_PARAM, TG_COMMAND_EDIT_PARAM, TG_COMMAND_DEL_PARAM:
							{
								msg := update.CallbackQuery.Message
								handler.HandleEditOutMessage(msg.Text, msg.MessageID)
							}
						}
						error_str = handler.ErrorStr
					}
				}
				bot.Send(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
			} else if update.Message != nil { // If we got a message
				if client == nil {
					// if no client found - add the new one to the pool
					client, err = clientpool.AddCID(tgid,
						update.Message.From.UserName,
						update.Message.From.FirstName,
						update.Message.From.LastName,
						GetLocale(update.Message.From.LanguageCode))
					check(err)
					bot.Send(PrepareInitCommands(tgid, client.GetLocale()))
				}

				comm, params := ParseCommand(update.Message.Text)
				handler := NewCommandHandler(bot, client, clientpool, &comm, params)

				// first check if this is the common command
				switch comm {
				case TG_COMMAND_START:
					{
						handler.HandleStart()
					}
				case TG_COMMAND_AUTH:
					{
						handler.HandleAuth()
					}
				case TG_COMMAND_LOGOUT:
					{
						handler.HandleLogOut()
					}
				case TG_COMMAND_TARG:
					{
						handler.HandleSettingsSetTarget(client.GetSettings())
					}
				case TG_COMMAND_FILTER:
					{
						handler.HandleSettingsSetFilter(client.GetSettings())
					}
				case TG_COMMAND_DEVS:
					{
						handler.HandleDevices()
					}
				case TG_COMMAND_GETRID:
					{
						handler.HandleGetRid()
					}
				case TG_COMMAND_SETTS:
					{
						handler.SendSettingsAsJson()
					}
				default:
					{
						// no. this is not the common command
						if update.Message.ReplyToMessage == nil { // this is not the reply. maybe some message for the host
							if update.Message.Text != "" {
								// maybe this is the response to the client state-change request
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
								default:
									{
										// user trying to send some message to the host
										// create the new message editor
										handler := NewHandler(bot, client)
										handler.SendMessageAsJson(&wc.OutMessageStruct{Msg: update.Message.Text}, update.Message.MessageID)
										error_str = handler.ErrorStr
									}
								}
							} else if update.Message.Photo != nil {
								// user trying to save the new media object
								// try to extract the new media from message and send it to the host
								// find the best media size
								bst := update.Message.Photo[0]
								for _, mipmap := range update.Message.Photo {
									if mipmap.FileSize < wcb_cfg.MaxMediaSize { // check for limit
										bst = mipmap
									}
								}

								file, err := bot.GetFile(tgbotapi.FileConfig{FileID: bst.FileID})
								if err == nil {
									fullpath := FormatFileUrl(wcb_cfg.BotToken, file.FilePath)
									go SendMediaToHost(client, fullpath)
								} else {
									error_str = ErrorToString(err)
								}
							} else {
								error_str = client.GetLocale().UnsupportedMsg
							}
						} else if len(update.Message.ReplyToMessage.Entities) > 1 { // check if this is the reply for some editor
							// extract json obj (editor state)
							cur_cmd, params, cur_obj, ok := ExtractJsonObjFromEntities(
								update.Message.ReplyToMessage.Text,
								update.Message.ReplyToMessage.Entities)

							if ok {
								// Yes, we have a message in the editor's business logic section
								handler := NewCommandHandler(bot, client, clientpool, &cur_cmd, params)
								obj, err := DecodeJSONMessageText(cur_obj) // check if there is outmessage object
								if err == nil {
									switch cur_cmd {
									case TG_COMMAND_ADD_PARAM:
										{
											handler.HandleOutMessageDoAddParam(obj, update.Message.Text)
										}
									case TG_COMMAND_EDIT_PARAM:
										{
											handler.HandleOutMessageDoEditParam(obj, update.Message.Text)
										}
									}
									if len(handler.ErrorStr) == 0 {
										handler.SendMessageAsJson(obj, -1)
									}
									error_str = handler.ErrorStr
								} else {
									_, err := DecodeJSONSettingsText(cur_obj) // check if there is settings object
									if err == nil {
										switch cur_cmd {
										case TG_COMMAND_TARG:
											{
												bot.Send(PrepareSetTarget(clientpool, client, update.Message.Text))
											}
										case TG_COMMAND_FILTER:
											{
												bot.Send(PrepareSetFilter(clientpool, client, update.Message.Text))
											}
										}
										if len(handler.ErrorStr) == 0 {
											handler.SendSettingsAsJson()
										}
										error_str = handler.ErrorStr
									} else {
										error_str = ErrorToString(err)
									}
								}
							}
						} else {
							error_str = client.GetLocale().UnsupportedMsg
						}
					}
				}
			}
			if len(error_str) > 0 && (tgid.chat_id > 0) {
				bot.Send(PrepareDoLog(tgid.chat_id,
					fmt.Sprintf(DefaultLocale().ErrorDetected, error_str)))
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
