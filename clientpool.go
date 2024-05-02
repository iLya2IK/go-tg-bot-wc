package main

import (
	"bytes"
	"container/list"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"sync"
	"time"

	sqlite "crawshaw.io/sqlite"
	sqlitex "crawshaw.io/sqlite/sqlitex"
	wc "github.com/ilya2ik/wcwebcamclient_go"
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
	StatusWaiting      PoolClientStatus = 1
	StatusWaitLogin    PoolClientStatus = 2
	StatusWaitPassword PoolClientStatus = 3
	StatusAuthorizing  PoolClientStatus = 4
	StatusAuthorized   PoolClientStatus = 0x100
)

type PoolClientSettings struct {
	Target string `json:"target"`
	Filter string `json:"filter"`

	filteregex *regexp.Regexp
}

func (sett *PoolClientSettings) CheckFilter(device string) bool {
	if sett.filteregex != nil {
		return sett.filteregex.MatchString(device)
	}
	return true
}

func (sett *PoolClientSettings) SetFilter(filter string) error {
	sett.Filter = filter
	return sett.UpdateFilter()
}

func (sett *PoolClientSettings) UpdateFilter() error {
	var err error
	sett.filteregex, err = regexp.Compile(sett.Filter)
	return err
}

/* PoolClient decl */

type PoolClient struct {
	mux    sync.Mutex
	status PoolClientStatus

	id        TgUserId
	user_name string
	account   *url.Userinfo
	setting   PoolClientSettings

	locale *LanguageStrings

	client *wc.WCClient
}

/* PoolClient impl */

func (c *PoolClient) GetSettings() *PoolClientSettings {
	return &c.setting
}

func (c *PoolClient) GetLocale() *LanguageStrings {
	return c.locale
}

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

func (c *PoolClient) GetID() TgUserId {
	return c.id
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

type PoolUpdateRecsListener interface {
	OnUpdateRecs(client *PoolClient, msgs []map[string]any) /* The request to update list of media records has been completed. The response has arrived. */
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

type WCUpdateType int

const (
	Message WCUpdateType = iota
	Media
	MediaData
	MediaMeta
)

type WCUpdate struct {
	Client *PoolClient
	Type   WCUpdateType
	Msg    *wc.MessageStruct
	Rec    *wc.MediaStruct
	Raw    map[string]any
	Data   *BufferReader
}

type PoolUpdate chan WCUpdate

/* LockableStmt decl */

type LockableStmt struct {
	mux  sync.Mutex
	stmt *sqlite.Stmt
}

/* LockableStmt impl */

func PrepareStmt(db *sqlite.Conn, sql string) (*LockableStmt, error) {
	res := LockableStmt{}
	var err error
	res.stmt, err = db.Prepare(sql)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (stmt *LockableStmt) bind(bindings map[int]any) error {
	for key, v := range bindings {
		switch reflect.TypeOf(v).Kind() {
		case reflect.Int64, reflect.Int32:
			stmt.stmt.BindInt64(key, v.(int64))
		case reflect.Float32, reflect.Float64:
			stmt.stmt.BindFloat(key, v.(float64))
		case reflect.String:
			stmt.stmt.BindText(key, v.(string))
		default:
			return fmt.Errorf("wrong type of binding %d, %v", key, v)
		}
	}
	return nil
}

func (stmt *LockableStmt) DoUpdate(bindings map[int]any) error {
	stmt.Lock()
	defer stmt.UnLock()

	stmt.bind(bindings)

	if _, err := stmt.stmt.Step(); err != nil {
		return err
	}

	return stmt.stmt.Reset()
}

func (stmt *LockableStmt) DoSelectRow(bindings map[int]any, cols []string) (map[string]string, error) {
	stmt.Lock()
	defer stmt.UnLock()

	stmt.bind(bindings)

	if hasRow, err := stmt.stmt.Step(); err != nil {
		return nil, err
	} else if hasRow {
		result := make(map[string]string)
		for _, rec := range cols {
			result[rec] = stmt.stmt.GetText(rec)
		}
		return result, stmt.stmt.Reset()
	}
	return nil, sqlitex.ErrNoResults
}

func (stmt *LockableStmt) Lock() {
	stmt.mux.Lock()
}

func (stmt *LockableStmt) UnLock() {
	stmt.mux.Unlock()
}

/* Pool decl */

type Pool struct {
	mux     sync.Mutex
	listmux sync.Mutex

	client_db *sqlite.Conn
	// Prepares
	adduser_stmt     *LockableStmt
	getuser_stmt     *LockableStmt
	upduser_stmt     *LockableStmt
	addlogin_stmt    *LockableStmt
	getlogin_stmt    *LockableStmt
	getstamps_stmt   *LockableStmt
	updrecstamp_stmt *LockableStmt
	updmsgstamp_stmt *LockableStmt

	initial_cfg *wc.WCClientConfig

	value     *list.List
	listeners *list.List

	updates PoolUpdate
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
		"CONSTRAINT \"fk_ext\" FOREIGN KEY (\"ext_user_id\", \"ext_chat_id\") "+
		"REFERENCES \"users\" (\"user_id\", \"chat_id\") on delete cascade,"+
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
		updates:     make(PoolUpdate, 128),
	})

	if pool.adduser_stmt, err = PrepareStmt(db,
		"with _ex_ as (select * from \"users\" where \"user_id\"=?1 and \"chat_id\" = ?2 limit 1)"+
			"replace into \"users\" "+
			"(\"user_id\", \"chat_id\", \"user_name\", \"user_first_name\", \"user_second_name\", \"last_start\", \"settings\") "+
			"values (?1, ?2, ?3, ?4, ?5, current_timestamp,"+
			"CASE WHEN EXISTS(select * from _ex_) THEN (select \"settings\" from _ex_) ELSE '{}' end);"); err != nil {
		return nil, err
	}
	if pool.upduser_stmt, err = PrepareStmt(db,
		"update \"users\" set \"settings\"=?3 where \"user_id\"=?1 and \"chat_id\"=?2;"); err != nil {
		return nil, err
	}
	if pool.getuser_stmt, err = PrepareStmt(db,
		"select * from \"users\" where \"user_id\"=?1 and \"chat_id\"=?2;"); err != nil {
		return nil, err
	}
	if pool.addlogin_stmt, err = PrepareStmt(db,
		"with _ex_ as (select * from \"logins\" where \"ext_user_id\"=?1 and \"ext_chat_id\" = ?2 and \"name\" = ?3 limit 1)"+
			"replace into \"logins\" "+
			"(\"ext_user_id\", \"ext_chat_id\", \"name\", \"pwd\", \"last_used\", \"msg_stamp\", \"rec_stamp\")"+
			"values (?1, ?2, ?3, ?4, current_timestamp, "+
			"CASE WHEN EXISTS(select * from _ex_) THEN (select \"msg_stamp\" from _ex_) ELSE '' end,"+
			"CASE WHEN EXISTS(select * from _ex_) THEN (select \"rec_stamp\" from _ex_) ELSE '' end);"); err != nil {
		return nil, err
	}
	if pool.getlogin_stmt, err = PrepareStmt(db,
		"select \"name\", \"pwd\", \"last_used\" from \"logins\" "+
			" where \"ext_user_id\"=?1 and \"ext_chat_id\"=?2 order by \"last_used\" desc;"); err != nil {
		return nil, err
	}
	if pool.updmsgstamp_stmt, err = PrepareStmt(db,
		"update \"logins\" set \"msg_stamp\"=?4 where "+
			"\"ext_user_id\"=?1 and \"ext_chat_id\"=?2 and \"name\"=?3;"); err != nil {
		return nil, err
	}
	if pool.updrecstamp_stmt, err = PrepareStmt(db,
		"update \"logins\" set \"rec_stamp\"=?4 where "+
			"\"ext_user_id\"=?1 and \"ext_chat_id\"=?2 and \"name\"=?3;"); err != nil {
		return nil, err
	}
	if pool.getstamps_stmt, err = PrepareStmt(db,
		"select \"msg_stamp\", \"rec_stamp\" from \"logins\" "+
			" where \"ext_user_id\"=?1 and \"ext_chat_id\"=?2 and \"name\"=?3;"); err != nil {
		return nil, err
	}

	return pool, nil
}

func (pool *Pool) NewPoolClient(cfg *wc.WCClientConfig, id TgUserId, un, fn, ln string, local *LanguageStrings) (*PoolClient, error) {
	c, err := pool.InitNewWCClient(cfg, un)
	if err != nil {
		return nil, err
	}

	new_pool_client := &(PoolClient{id: id, user_name: un, account: &url.Userinfo{}, locale: local, client: c})

	pool.PushBack(new_pool_client)

	return new_pool_client, nil
}

func (pool *Pool) InitNewWCClient(cfg *wc.WCClientConfig, un string) (*wc.WCClient, error) {
	var new_cfg *wc.WCClientConfig = wc.ClientCfgNew()
	new_cfg.AssignFrom(cfg)
	new_cfg.SetDevice(un)

	c, err := wc.ClientNew(new_cfg)
	if err != nil {
		return nil, err
	}
	c.SetNeedToSync(true)
	c.SetLstMsgStampToSyncPoint()
	c.SetOnAuthSuccess(pool.internalAuthSuccess)
	c.SetOnAddLog(pool.internalOnLog)
	c.SetOnConnected(pool.internalOnClientStateChange)
	c.SetOnUpdateMsgs(pool.internalOnUpdateMsgs)
	c.SetOnUpdateRecords(pool.internalOnUpdateRecords)
	c.SetOnUpdateDevices(pool.internalOnUpdateDevices)
	c.SetOnReqRecordData(pool.internalOnRecData)

	return c, nil
}

func (pool *Pool) GetPoolTimer() PoolUpdate {
	go func() {
		for {
			pool.DoForAll(func(c *PoolClient) bool {
				if c.client.Working() && (c.GetStatus()&StatusAuthorized > 0) {
					return true
				}
				return false
			}, pool.Update)

			time.Sleep(time.Second * 5)
		}
	}()

	return pool.updates
}

func (pool *Pool) dbAddCID(id TgUserId, un, fn, ln string) (PoolClientSettings, error) {
	const SETTINGS_COL = "settings"
	var sett PoolClientSettings
	cols, err := pool.getuser_stmt.DoSelectRow(
		map[int]any{1: id.user_id, 2: id.chat_id},
		[]string{SETTINGS_COL})
	if err != nil && (err != sqlitex.ErrNoResults) {
		return sett, err
	}

	if cols != nil {
		err = json.Unmarshal([]byte(cols[SETTINGS_COL]), &sett)
		if len(sett.Target) == 0 || err != nil {
			sett.Target = ALL_DEVICES
		}
		if len(sett.Filter) == 0 || err != nil {
			sett.Filter = ALL_FILTER
		}
	}
	sett.UpdateFilter()

	err = pool.adduser_stmt.DoUpdate(
		map[int]any{
			1: id.user_id,
			2: id.chat_id,
			3: un,
			4: fn,
			5: ln})
	return sett, err
}

func (pool *Pool) AddCID(id TgUserId, un, fn, ln string, locale *LanguageStrings) (*PoolClient, error) {

	sett, err := pool.dbAddCID(id, un, fn, ln)
	if err != nil {
		return nil, err
	}

	client, err := pool.NewPoolClient(pool.initial_cfg, id, un, fn, ln, locale)
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
	if client.GetWCClient().GetClientStatus() == wc.StateDisconnected {
		c, err := pool.InitNewWCClient(pool.initial_cfg, client.GetUserName())
		if err != nil {
			return err
		}
		client.client = c
	}
	if err := client.authorize(); err != nil {
		return err
	}
	pw, _ := client.account.Password()
	err := pool.addlogin_stmt.DoUpdate(
		map[int]any{
			1: client.id.user_id,
			2: client.id.chat_id,
			3: client.account.Username(),
			4: pw})
	return err
}

func (pool *Pool) UpdateDevices(client *PoolClient) error {
	return client.client.UpdateDevices(client)
}

func (pool *Pool) Update(client *PoolClient) {
	client.client.UpdateMsgs(client)
	client.client.UpdateRecords(client)
}

func (pool *Pool) UpdateMessages(client *PoolClient) {
	client.client.UpdateMsgs(client)
}

func (pool *Pool) UpdateRecords(client *PoolClient) {
	client.client.UpdateRecords(client)
}

func (pool *Pool) DownloadRid(client *PoolClient, rid int64) {
	client.client.RequestRecord(int(rid), &(RequestedRecord{client: client, rid: rid}))
}

func (pool *Pool) updateClientSettings(client *PoolClient) error {
	json_str, err := json.Marshal(client.setting)
	if err != nil {
		return err
	}

	err = pool.upduser_stmt.DoUpdate(
		map[int]any{
			1: client.id.user_id,
			2: client.id.chat_id,
			3: string(json_str)})
	return err
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
	err := client.setting.SetFilter(value)
	if err != nil {
		return err
	}
	return pool.updateClientSettings(client)
}

func (pool *Pool) internalAuthSuccess(tsk wc.ITask) {
	cl := pool.ByWCRef(tsk.GetClient())
	pool.broadcastEvent(cl, intrfAuth, []any{})

	if cl != nil {
		const MSG_STAMP_COL = "msg_stamp"
		const REC_STAMP_COL = "rec_stamp"

		cols, err := pool.getstamps_stmt.DoSelectRow(
			map[int]any{
				1: cl.id.user_id,
				2: cl.id.chat_id,
				3: cl.account.Username()},
			[]string{MSG_STAMP_COL, REC_STAMP_COL})
		if err != nil {
			return
		}

		if cols != nil {
			cl.GetWCClient().SetLstMsgStamp(cols[MSG_STAMP_COL])
			cl.GetWCClient().SetLstRecStamp(cols[REC_STAMP_COL])
		}
	}
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
	if cl != nil {
		for _, msg := range jsonresult {
			var msg_r wc.MessageStruct
			msg_r.JSONDecode(msg)

			upd := WCUpdate{
				Client: cl,
				Type:   Message,
				Msg:    &msg_r,
				Raw:    msg,
			}
			pool.updates <- upd
		}

		pool.updmsgstamp_stmt.DoUpdate(
			map[int]any{
				1: cl.id.user_id,
				2: cl.id.chat_id,
				3: cl.account.Username(),
				4: cl.GetWCClient().GetLstMsgStamp()})
	}
}

func (pool *Pool) internalOnUpdateRecords(tsk wc.ITask, jsonresult []map[string]any) {
	cl := tsk.GetUserData().(*PoolClient)
	pool.broadcastEvent(cl, intrfUpdateRecs, []any{jsonresult})
	if cl != nil {
		for _, rec := range jsonresult {
			var rec_r wc.MediaStruct
			rec_r.JSONDecode(rec)

			upd := WCUpdate{
				Client: cl,
				Type:   Media,
				Rec:    &rec_r,
				Raw:    rec,
			}
			pool.updates <- upd
		}

		pool.updrecstamp_stmt.DoUpdate(
			map[int]any{
				1: cl.id.user_id,
				2: cl.id.chat_id,
				3: cl.account.Username(),
				4: cl.GetWCClient().GetLstRecStamp()})
	}
}

func (pool *Pool) internalOnUpdateDevices(tsk wc.ITask, jsonresult []map[string]any) {
	cl := tsk.GetUserData().(*PoolClient)
	pool.broadcastEvent(cl, intrfUpdateDevices, []any{jsonresult})
}

type RequestedRecord struct {
	rid    int64
	client *PoolClient
}

type BufferReader struct {
	name string
	id   int64
	data *bytes.Buffer
}

func (reader *BufferReader) IsEmpty() bool {
	if reader.data == nil {
		return true
	}
	return reader.data.Len() == 0
}

func (reader *BufferReader) GetId() int64 {
	return reader.id
}

// NeedsUpload shows if the file needs to be uploaded.
func (reader *BufferReader) NeedsUpload() bool {
	return true
}

// UploadData gets the file name and an `io.Reader` for the file to be uploaded. This
// must only be called when the file needs to be uploaded.
func (reader *BufferReader) UploadData() (string, io.Reader, error) {
	return reader.name, reader.data, nil
}

// SendData gets the file data to send when a file does not need to be uploaded. This
// must only be called when the file does not need to be uploaded.
func (reader *BufferReader) SendData() string {
	return fmt.Sprintf("Cant upload %s", reader.name)
}

func (pool *Pool) internalOnRecData(tsk wc.ITask, data *bytes.Buffer) {
	cl := tsk.GetUserData().(*RequestedRecord)
	if cl != nil {
		dt := BufferReader{name: fmt.Sprintf("rid%d.png", cl.rid), id: cl.rid}
		if data.Len() < 64 {
			dt.data = nil
		} else {
			dt.data = data
		}
		upd := WCUpdate{
			Client: cl.client,
			Type:   MediaData,
			Data:   &dt,
		}
		pool.updates <- upd
	}
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
	const NAME_COL = "name"
	const PWD_COL = "pwd"

	cols, err := pool.getlogin_stmt.DoSelectRow(
		map[int]any{1: client.id.user_id,
			2: client.id.chat_id},
		[]string{NAME_COL, PWD_COL})
	if err != nil && (err != sqlitex.ErrNoResults) {
		return "", "", err
	}

	if cols != nil {
		un := cols[NAME_COL]
		pwd := cols[PWD_COL]
		return un, pwd, nil
	}
	return "", "", nil
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
	intrfUpdateRecs
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
		case intrfUpdateRecs:
			{
				auth, ok := cl.(PoolUpdateRecsListener)
				if ok {
					auth.OnUpdateRecs(client, params[0].([]map[string]any))
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
