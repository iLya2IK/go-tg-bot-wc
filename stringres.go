package main

type LanguageStrings struct {
	IETFCode            string
	Greetings           string
	ClientAuthorized    string
	ClientStatusChanged string
	SetTarget           string
	SetTargetAll        string
	AlreadyAuthorized   string
	NotAuthorized       string
	TargetUpdated       string
	CommandStart        string
	CommandAuth         string
	CommandSett         string
	CommandLogOut       string
	CommandTarget       string
	CommandFilter       string
	CommandDevs         string
	AddParam            string
	EditParam           string
	DeleteParam         string
	SendMsgTo           string
	ToJSON              string
	GetMedia            string
	IncomingMsg         string
	IncomingMedia       string
	NoSuchMediaRID      string
	MediaRID            string
	EmptyTarget         string
	NoParams            string
	EmptyCallback       string
	SetNewParamName     string
	SetNewParamValue    string
	LoginPrompt         string
	PasswordPrompt      string
	TargetPrompt        string
	FilterPrompt        string
	FilterUpdated       string
	WrongParamName      string
	UnsupportedMsg      string
	ErrorDetected       string
}

var EN_STRINGS = LanguageStrings{
	IETFCode: "en",
	Greetings: "<b>Hello, my name is tgTowc_bot</b>\n" +
		"<a href=\"%s\">%s</a> to start working with the bot",
	ClientAuthorized:    "Client authorized\n New SID : %s",
	ClientStatusChanged: "Client status changed\n %s",
	SetTarget:           "Set target to %s",
	SetTargetAll:        "Set target to all",
	AlreadyAuthorized:   "<b>%s</b> is already authorized",
	NotAuthorized: "<b>%s</b> is not authorized\n" +
		"<a href=\"%s\">%s</a> to start working with the bot",
	TargetUpdated: "Target updated: %s",
	CommandStart:  "Start this bot",
	CommandAuth:   "Try to login as the user",
	CommandSett:   "View all settings options",
	CommandLogOut: "Log out",
	CommandTarget: "Change the target device",
	CommandFilter: "Change the incoming filter for devices",
	CommandDevs:   "Get list of all online devices",

	AddParam:    "Add new param",
	EditParam:   "Edit %s value",
	DeleteParam: "Delete %s",
	SendMsgTo:   "Send to %s",
	ToJSON:      "To JSON",
	GetMedia:    "Get Media",

	IncomingMsg:   "New message",
	IncomingMedia: "New media",

	NoSuchMediaRID: "Error: No such rid %d",
	MediaRID:       "Media rid %d",

	NoParams:         "Parameters not received",
	EmptyTarget:      "Empty target",
	EmptyCallback:    "Empty callback",
	SetNewParamName:  "Set new param name",
	SetNewParamValue: "Set new param value",
	LoginPrompt:      "Your <b>login</b>:",
	PasswordPrompt:   "Your <b>password</b>:",
	TargetPrompt: "Set <b>target</b> device.\nTo get the list of online devices use the\n " +
		"<a href=\"%s\">%s</a> request.\n" +
		"To get all messages from all devices use:\n<b>all</b>.\nCurrent target: <b>%s</b>",
	FilterPrompt: "Describe the incoming message filter by device name.\n" +
		"The filter is set by the regexp expression.\nCurrent filter: <b>%s</b>",

	FilterUpdated:  "Filter updated: %s",
	WrongParamName: "Wrong param name: %s",
	UnsupportedMsg: "Unsupported message format",
	ErrorDetected: "<pre>Error detected</pre>\n" +
		"<pre>%s</pre>\nTry to use the bot service properly",
}

func GetLocale(locale string) *LanguageStrings {
	return &EN_STRINGS
}

func DefaultLocale() *LanguageStrings {
	return &EN_STRINGS
}
