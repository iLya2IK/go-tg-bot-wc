package main

import (
	"container/list"
	"encoding/json"
	"net/url"
	"sync"

	sqlite "crawshaw.io/sqlite"
	sqlitex "crawshaw.io/sqlite/sqlitex"
	wc "github.com/ilya2ik/wcwebcamclient_go/lib"
)

type TgUserId struct {
	user_id int64
	chat_id int64
}

func (id TgUserId) Compare(src TgUserId) int {
	if id.user_id < src.user_id {
		return -1
	} else if id.user_id > src.user_id {
		return 1
	} else {
		if id.chat_id < src.chat_id {
			return -1
		} else if id.chat_id > src.chat_id {
			return 1
		}
	}
	return 0
}

type PoolClientStatus int

const (
	StatusWaiting       PoolClientStatus = 1
	StatusWaitLogin     PoolClientStatus = 2
	StatusWaitPassword  PoolClientStatus = 3
	StatusAuthorizing   PoolClientStatus = 4
	StatusAuthorized    PoolClientStatus = 0x100
	StatusWaitSetTarget PoolClientStatus = 0x101
	StatusWaitSetFilter PoolClientStatus = 0x102
)

type PoolClientSettings struct {
	Target string `json:"target"`
	Filter string `json:"filter"`
}

/* PoolClient decl */

type PoolClient struct {
	mux    sync.Mutex
	status PoolClientStatus

	id        TgUserId
	user_name string
	account   *url.Userinfo
	setting   PoolClientSettings

	client *wc.WCClient
}

/* PoolClient impl */

func (c *PoolClient) GetStatus() PoolClientStatus {
	c.mux.Lock()
	defer c.mux.Unlock()

	return c.status
}

func (c *PoolClient) SetStatus(ns PoolClientStatus) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.status = ns
}

func (c *PoolClient) SetLogin(un string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.account = url.User(un)
}

func (c *PoolClient) SetPwd(pwd string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.account = url.UserPassword(c.account.Username(), pwd)
}

func (c *PoolClient) CanAutoAuthorize() bool {
	c.mux.Lock()
	defer c.mux.Unlock()

	if c.account != nil {
		_, pwdok := c.account.Password()
		if (len(c.account.Username()) > 0) && (pwdok) {
			return true
		}
	}
	return false
}

func (c *PoolClient) IsAuthorized() bool {
	if c.client == nil {
		return false
	}
	return (c.GetStatus()&StatusAuthorized > 0) || (c.GetStatus() == StatusAuthorizing)
}

func (c *PoolClient) Locate(id TgUserId) bool {
	return (c.id.Compare(id) == 0)
}

func (c *PoolClient) GetChatID() int64 {
	return c.id.chat_id
}

func (c *PoolClient) GetUserName() string {
	return c.user_name
}

func (c *PoolClient) GetWCClient() *wc.WCClient {
	return c.client
}

func (c *PoolClient) GetTarget() string {
	return c.setting.Target
}

func (c *PoolClient) GetFilter() string {
	return c.setting.Filter
}

func (c *PoolClient) authorize() error {
	if !c.client.Working() {
		err := c.client.Start()
		if err != nil {
			return err
		}
	}
	c.SetStatus(StatusAuthorizing)
	c.mux.Lock()
	defer c.mux.Unlock()
	pwd, _ := c.account.Password()
	return c.client.Auth(c.account.Username(), pwd)
}

type PoolAuthListener interface {
	OnSuccessAuth(client *PoolClient) /* Successful authorization. */
}

type PoolConnListener interface {
	OnConnected(client *PoolClient, status wc.ClientStatus) /* The connection state has been changed. */
}

type PoolAddLogListener interface {
	OnAddLog(client *PoolClient, value string) /* Added new log entry. */
}

type PoolUpdateMsgsListener interface {
	OnUpdateMsgs(client *PoolClient, msgs []map[string]any) /* The request to update list of messages has been completed. The response has arrived. */
}

type PoolUpdateDevicesListener interface {
	OnUpdateDevices(client *PoolClient, devices []map[string]any) /* The request to update list of online devices has been completed. The response has arrived. */
}

//	onDisconnect  NotifyEventFunc     /* Client has been disconnected. */
//	onSIDSetted   StringNotifyFunc    /* The session id has been changed. */
//	onAddLog      StringNotifyFunc    /* Added new log entry. */

/* streams block */
//	onAfterLaunchInStream  TaskNotifyFunc /* Incoming stream started. */
//	onAfterLaunchOutStream TaskNotifyFunc /* Outgoing stream started. */
//	onSuccessIOStream      TaskNotifyFunc /* IO stream terminated for some reason. */

/* data blobs block */
//	onSuccessSaveRecord    TaskNotifyFunc      /* The request to save the media record has been completed. The response has arrived. */
//	onSuccessRequestRecord DataNotifyEventFunc /* The request to get the media record has been completed. The response has arrived. */

/* JSON block */
//	onSuccessUpdateRecords     JSONArrayNotifyEventFunc /* The request to update list of records has been completed. The response has arrived. */
//	onSuccessUpdateStreams     JSONArrayNotifyEventFunc /* The request to update list of streaming devices has been completed. The response has arrived. */
//	onSuccessGetConfig         JSONArrayNotifyEventFunc /* The request to get actual config has been completed. The response has arrived. */
//	onSuccessSendMsg           TaskNotifyFunc           /* The request to send message has been completed. The response has arrived.  */
//	onSuccessSetConfig         TaskNotifyFunc           /* The request to set new client configuration has been completed. The response has arrived.  */
//	onSuccessRequestRecordMeta JSONNotifyEventFunc      /* The request to get metadata for the media record has been completed. The response has arrived. */
//	onSuccessDeleteRecords     TaskNotifyFunc           /* The request to delete records has been completed. The response has arrived. */

/* Pool decl */

type Pool struct {
	mux     sync.Mutex
	stmtmux sync.Mutex
	listmux sync.Mutex

	client_db *sqlite.Conn
	// Prepares
	adduser_stmt  *sqlite.Stmt
	getuser_stmt  *sqlite.Stmt
	upduser_stmt  *sqlite.Stmt
	addlogin_stmt *sqlite.Stmt
	getlogin_stmt *sqlite.Stmt

	initial_cfg *wc.WCClientConfig

	value     *list.List
	listeners *list.List
}

/* Pool impl */

func NewPool(client_db_loc string, cfg *wc.WCClientConfig) (*Pool, error) {
	db, err := sqlite.OpenConn(client_db_loc, sqlite.SQLITE_OPEN_READWRITE|sqlite.SQLITE_OPEN_CREATE)
	if err != nil {
		return nil, err
	}
	err = sqlitex.Exec(db, "create table if not exists \"users\" ("+
		"\"user_id\" int,"+
		"\"chat_id\" int,"+
		"\"user_name\" text,"+
		"\"user_first_name\" text,"+
		"\"user_second_name\" text,"+
		"\"last_start\" text default (current_timestamp),"+
		"\"settings\" text default ('{}'),"+
		"unique (\"user_id\", \"chat_id\"));", nil)
	if err != nil {
		return nil, err
	}
	err = sqlitex.Exec(db, "create table if not exists \"logins\" ("+
		"\"ext_user_id\" int not null,"+
		"\"ext_chat_id\" int not null,"+
		"\"name\" text not null,"+
		"\"pwd\" text,"+
		"\"last_used\" text default (current_timestamp),"+
		"\"msg_stamp\" text default '',"+
		"\"rec_stamp\" text default '',"+
		"CONSTRAINT fk_ext FOREIGN KEY (ext_user_id, ext_chat_id) REFERENCES users (user_id, chat_id) on delete cascade,"+
		"unique (\"ext_user_id\", \"ext_chat_id\", \"name\"));", nil)
	if err != nil {
		return nil, err
	}

	pool := &(Pool{
		//terminate:  make(chan bool, 2),
		initial_cfg: cfg,
		client_db:   db,
		value:       list.New(),
		listeners:   list.New(),
	})

	stmt, err := db.Prepare("replace into \"users\" " +
		"(\"user_id\", \"chat_id\", \"user_name\", \"user_first_name\", \"user_second_name\", \"last_start\") " +
		"values (?1, ?2, ?3, ?4, ?5, current_timestamp);")
	if err != nil {
		return nil, err
	}
	pool.adduser_stmt = stmt
	stmt, err = db.Prepare("update \"users\" set \"settings\"=?3 where \"user_id\"=?1 and \"chat_id\"=?2;")
	if err != nil {
		return nil, err
	}
	pool.upduser_stmt = stmt
	stmt, err = db.Prepare("select * from \"users\" where \"user_id\"=?1 and \"chat_id\"=?2;")
	if err != nil {
		return nil, err
	}
	pool.getuser_stmt = stmt
	stmt, err = db.Prepare("replace into \"logins\" " +
		"(\"ext_user_id\", \"ext_chat_id\", \"name\", \"pwd\", \"last_used\") " +
		"values (?1, ?2, ?3, ?4, current_timestamp);")
	if err != nil {
		return nil, err
	}
	pool.addlogin_stmt = stmt
	stmt, err = db.Prepare("select \"name\", \"pwd\", \"last_used\" from \"logins\" " +
		" where \"ext_user_id\"=?1 and \"ext_chat_id\"=?2 order by \"last_used\" desc;")
	if err != nil {
		return nil, err
	}
	pool.getlogin_stmt = stmt

	return pool, nil
}

func (pool *Pool) NewPoolClient(cfg *wc.WCClientConfig, id TgUserId, un, fn, ln string) (*PoolClient, error) {
	var new_cfg *wc.WCClientConfig = wc.ClientCfgNew()
	new_cfg.AssignFrom(cfg)
	new_cfg.SetDevice(un)

	c, err := wc.ClientNew(new_cfg)
	if err != nil {
		return nil, err
	}
	c.SetOnAuthSuccess(pool.internalAuthSuccess)
	c.SetOnAddLog(pool.internalOnLog)
	c.SetOnConnected(pool.internalOnClientStateChange)
	c.SetOnUpdateMsgs(pool.internalOnUpdateMsgs)
	c.SetOnUpdateDevices(pool.internalOnUpdateDevices)

	new_pool_client := &(PoolClient{id: id, user_name: un, account: &url.Userinfo{}, client: c})

	pool.PushBack(new_pool_client)

	return new_pool_client, nil
}

func (pool *Pool) dbAddCID(id TgUserId, un, fn, ln string) (PoolClientSettings, error) {
	pool.stmtmux.Lock()
	defer pool.stmtmux.Unlock()

	var sett PoolClientSettings
	pool.getuser_stmt.BindInt64(1, id.user_id)
	pool.getuser_stmt.BindInt64(2, id.chat_id)
	if hasRow, err := pool.getuser_stmt.Step(); err != nil {
		return sett, err
	} else if hasRow {
		sett_str := pool.getuser_stmt.GetText("settings")
		err = json.Unmarshal([]byte(sett_str), &sett)
		if err != nil {
			pool.getuser_stmt.Reset()
			return sett, err
		}
		if len(sett.Target) == 0 {
			sett.Target = ALL_DEVICES
		}
		if len(sett.Filter) == 0 {
			sett.Filter = ALL_FILTER
		}
	}
	err := pool.getuser_stmt.Reset()
	if err != nil {
		return sett, err
	}
	pool.adduser_stmt.BindInt64(1, id.user_id)
	pool.adduser_stmt.BindInt64(2, id.chat_id)
	pool.adduser_stmt.BindText(3, un)
	pool.adduser_stmt.BindText(4, fn)
	pool.adduser_stmt.BindText(5, ln)
	if _, err := pool.adduser_stmt.Step(); err != nil {
		return sett, err
	}
	err = pool.adduser_stmt.Reset()
	return sett, err
}

func (pool *Pool) AddCID(id TgUserId, un, fn, ln string) (*PoolClient, error) {

	sett, err := pool.dbAddCID(id, un, fn, ln)
	if err != nil {
		return nil, err
	}

	client, err := pool.NewPoolClient(pool.initial_cfg, id, un, fn, ln)
	if err != nil {
		return nil, err
	}
	client.setting = sett

	hun, hpwd, err := pool.GetLastLoginData(client)
	if err != nil {
		return nil, err
	}

	if len(hun) > 0 && len(hpwd) > 0 {
		client.account = url.UserPassword(hun, hpwd)
	}

	return client, nil
}

func (pool *Pool) Authorize(client *PoolClient) error {
	if err := client.authorize(); err != nil {
		return err
	}

	pool.stmtmux.Lock()
	defer pool.stmtmux.Unlock()

	pool.addlogin_stmt.BindInt64(1, client.id.user_id)
	pool.addlogin_stmt.BindInt64(2, client.id.chat_id)
	pool.addlogin_stmt.BindText(3, client.account.Username())
	pw, _ := client.account.Password()
	pool.addlogin_stmt.BindText(4, pw)
	if _, err := pool.addlogin_stmt.Step(); err != nil {
		return err
	}
	return pool.addlogin_stmt.Reset()
}

func (pool *Pool) UpdateDevices(client *PoolClient) error {
	return client.client.UpdateDevices(client)
}

func (pool *Pool) updateClientSettings(client *PoolClient) error {
	json_str, err := json.Marshal(client.setting)
	if err != nil {
		return err
	}

	pool.stmtmux.Lock()
	defer pool.stmtmux.Unlock()

	pool.upduser_stmt.BindInt64(1, client.id.user_id)
	pool.upduser_stmt.BindInt64(2, client.id.chat_id)
	pool.upduser_stmt.BindText(3, string(json_str))
	if _, err := pool.upduser_stmt.Step(); err != nil {
		return err
	}
	return pool.upduser_stmt.Reset()
}

const ALL_DEVICES = "all"
const ALL_FILTER = ".*"

func (pool *Pool) SetClientTarget(client *PoolClient, value string) error {
	if len(value) == 0 {
		value = ALL_DEVICES
	}
	client.setting.Target = value
	return pool.updateClientSettings(client)
}

func (pool *Pool) SetClientFilter(client *PoolClient, value string) error {
	if len(value) == 0 {
		value = ALL_FILTER
	}
	client.setting.Filter = value
	return pool.updateClientSettings(client)
}

func (pool *Pool) internalAuthSuccess(tsk wc.ITask) {
	cl := pool.ByWCRef(tsk.GetClient())
	pool.broadcastEvent(cl, intrfAuth, []any{})
}

func (pool *Pool) internalOnLog(client *wc.WCClient, value string) {
	cl := pool.ByWCRef(client)
	pool.broadcastEvent(cl, intrfAddLog, []any{value})
}

func (pool *Pool) internalOnClientStateChange(client *wc.WCClient, status wc.ClientStatus) {
	cl := pool.ByWCRef(client)
	if cl != nil {
		switch status {
		case wc.StateDisconnected,
			wc.StateConnectedWrongSID,
			wc.StateConnected:
			{
				cl.SetStatus(StatusWaiting)
			}
		}
	}
	pool.broadcastEvent(cl, intrfConn, []any{status})
}

func (pool *Pool) internalOnUpdateMsgs(tsk wc.ITask, jsonresult []map[string]any) {
	cl := tsk.GetUserData().(*PoolClient)
	pool.broadcastEvent(cl, intrfUpdateMsgs, []any{jsonresult})
}

func (pool *Pool) internalOnUpdateDevices(tsk wc.ITask, jsonresult []map[string]any) {
	cl := tsk.GetUserData().(*PoolClient)
	pool.broadcastEvent(cl, intrfUpdateDevices, []any{jsonresult})
}

func (pool *Pool) PushBack(str *PoolClient) {
	pool.mux.Lock()
	defer pool.mux.Unlock()

	pool.value.PushBack(str)
}

func (pool *Pool) NotEmpty() bool {
	pool.mux.Lock()
	defer pool.mux.Unlock()

	return pool.value.Len() > 0
}

func (pool *Pool) Pop() *PoolClient {
	pool.mux.Lock()
	defer pool.mux.Unlock()

	el := pool.value.Front()
	if el != nil {
		return pool.value.Remove(el).(*PoolClient)
	} else {
		return nil
	}
}

func (pool *Pool) Clear() {
	pool.mux.Lock()
	defer pool.mux.Unlock()

	pool.value = list.New()
}

func (pool *Pool) BySID(sid string) *PoolClient {
	pool.mux.Lock()
	defer pool.mux.Unlock()

	for e := pool.value.Front(); e != nil; e = e.Next() {
		var cl = e.Value.(*PoolClient)
		if cl.client.GetSID() == sid {
			return cl
		}
	}
	return nil
}

func (pool *Pool) ByWCRef(ref *wc.WCClient) *PoolClient {
	pool.mux.Lock()
	defer pool.mux.Unlock()

	for e := pool.value.Front(); e != nil; e = e.Next() {
		var cl = e.Value.(*PoolClient)
		if cl.client == ref {
			return cl
		}
	}
	return nil
}

func (pool *Pool) ByUCID(id TgUserId) *PoolClient {
	pool.mux.Lock()
	defer pool.mux.Unlock()

	for e := pool.value.Front(); e != nil; e = e.Next() {
		var cl = e.Value.(*PoolClient)
		if cl.Locate(id) {
			return cl
		}
	}
	return nil
}

func (pool *Pool) GetLastLoginData(client *PoolClient) (string, string, error) {
	pool.stmtmux.Lock()
	defer pool.stmtmux.Unlock()

	pool.getlogin_stmt.BindInt64(1, client.id.user_id)
	pool.getlogin_stmt.BindInt64(2, client.id.chat_id)
	if hasRow, err := pool.getlogin_stmt.Step(); err != nil {
		return "", "", err
	} else if !hasRow {
		err := pool.getlogin_stmt.Reset()
		return "", "", err
	}
	un := pool.getlogin_stmt.GetText("name")
	pwd := pool.getlogin_stmt.GetText("pwd")
	err := pool.getlogin_stmt.Reset()
	if err != nil {
		return "", "", err
	}
	return un, pwd, nil
}

type FilterPoolFunc func(c *PoolClient) bool
type DoPoolFunc func(c *PoolClient)

func (c *Pool) DoForAll(filter FilterPoolFunc, doex DoPoolFunc) {
	if doex == nil {
		return
	}

	c.mux.Lock()
	defer c.mux.Unlock()

	for e := c.value.Front(); e != nil; e = e.Next() {
		// do something with e.Value
		var cl = e.Value.(*PoolClient)
		if filter == nil || filter(cl) {
			doex(cl)
		}
	}
}

func (c *Pool) PushListener(listener any) {
	c.listmux.Lock()
	defer c.listmux.Unlock()

	c.listeners.PushBack(listener)
}

type listenerInterface int

const (
	intrfAuth listenerInterface = iota
	intrfConn
	intrfAddLog
	intrfUpdateMsgs
	intrfUpdateDevices
)

func (c *Pool) broadcastEvent(client *PoolClient, intf listenerInterface, params []any) {
	if client == nil {
		return
	}

	c.listmux.Lock()
	defer c.listmux.Unlock()

	for e := c.listeners.Front(); e != nil; e = e.Next() {
		// do something with e.Value
		var cl = e.Value
		switch intf {
		case intrfAuth:
			{
				auth, ok := cl.(PoolAuthListener)
				if ok {
					client.SetStatus(StatusAuthorized)
					auth.OnSuccessAuth(client)
				}
			}
		case intrfConn:
			{
				auth, ok := cl.(PoolConnListener)
				if ok {
					auth.OnConnected(client, params[0].(wc.ClientStatus))
				}
			}
		case intrfAddLog:
			{
				auth, ok := cl.(PoolAddLogListener)
				if ok {
					auth.OnAddLog(client, params[0].(string))
				}
			}
		case intrfUpdateMsgs:
			{
				auth, ok := cl.(PoolUpdateMsgsListener)
				if ok {
					auth.OnUpdateMsgs(client, params[0].([]map[string]any))
				}
			}
		case intrfUpdateDevices:
			{
				auth, ok := cl.(PoolUpdateDevicesListener)
				if ok {
					auth.OnUpdateDevices(client, params[0].([]map[string]any))
				}
			}
		}

	}
}
