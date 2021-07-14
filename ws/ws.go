package ws

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
)

// Client 用户类
type Client struct {
	ID		string
	Socket	*websocket.Conn
	Send	chan []byte
}

// Broadcast 广播类，包含广播内容和源用户（用于进行反馈）
type Broadcast struct {
	Client		*Client
	Message		[]byte
}

// ClientManager 用户管理
type ClientManager struct {
	Clients		map[string]*Client
	Broadcast	chan *Broadcast
	Reply		chan *Client
	Register	chan *Client
	Unregister	chan *Client
}

// Message 信息转json（包括：发送者、接受者、内容） 改进：添加头像的url，对发送者身份进行验证（token验证）
type Message struct {
	Sender		string		`json:"sender,omitempty"`
	Recipient	string		`json:"recipient,omitempty"`
	Content		string		`json:"content,omitempty"`
}

var Manager = ClientManager{
	Clients:    make(map[string]*Client),		// 参加连接的用户，出于性能考虑，需要设置最大连接数
	Broadcast:  make(chan *Broadcast),
	Register:   make(chan *Client),
	Reply:		make(chan *Client),
	Unregister: make(chan *Client),
}

// Start 项目运行前, 协程开启start -> go Manager.Start()
func (manager *ClientManager) Start() {
	for {
		log.Println("<---监听管道通信--->")
		select {
			// 建立连接
			case conn := <-Manager.Register:
				log.Println("建立新连接:%v", conn.ID)
				Manager.Clients[conn.ID] = conn		// 将此连接加入到管理器中，ID分类
				jsonMessage, _ := json.Marshal(&Message{Content: "已连接至服务器"})
				conn.Send <- jsonMessage
			// 断开连接
			case conn := <-Manager.Unregister:
				log.Println("连接关闭:%v", conn.ID)
				if _, ok := Manager.Clients[conn.ID]; ok {
					jsonMessage, _ := json.Marshal(&Message{Content: "连接已断开"})
					conn.Send <- jsonMessage
					close(conn.Send)
					delete(Manager.Clients, conn.ID)
				}
			//// 信息反馈
			//case conn := <-Manager.Reply:
			//	log.Println("消息成功发送反馈:%v", conn.ID)
			//	jsonMessage, _ := json.Marshal(&Message{Content: "消息成功发送反馈"})
			//	conn.Send <- jsonMessage
			// 广播信息
			case broadcast := <-Manager.Broadcast:
				message := broadcast.Message
				MessageStruct := Message{}
				json.Unmarshal(message, &MessageStruct)
				flag := false			// 默认对方不在线
				for id, conn := range Manager.Clients {
					// 向！所有！ID正确的连接发送广播信息
					if id != creatId(MessageStruct.Recipient, MessageStruct.Sender){
						continue
					}
					select {
						case conn.Send <- message:
							// 向对方发送信息成功
							flag = true
						default:
							// 收不到消息就关了他
							close(conn.Send)
							delete(Manager.Clients, conn.ID)
						}
				}
				if flag {
					log.Println("对方在线应答")
					// 写入redis，设为已读
					//log.Println("写入redis")

					//Manager.Reply <- broadcast.Client
				}else {
					log.Println("对方不在线应答")
					// 写入redis，设为未读
				}
		}
	}
}

// creatId 生成房间号。相当于把聊天室改为单向发送广播
func creatId(uid,touid string) string {
	return uid+"->"+touid
}

// Read connect监听用户消息，并向指定房间广播
func (c *Client) Read() {
	defer func() {
		Manager.Unregister <- c
		c.Socket.Close()
	}()

	for {
		c.Socket.PongHandler()
		_, message, err := c.Socket.ReadMessage()
		if err != nil {
			// 连接失败叫客户端关闭连接
			Manager.Unregister <- c
			c.Socket.Close()
			break
		}
		log.Println(c.ID, "发送信息:", string(message))
		Manager.Broadcast <- &Broadcast{
			Client:  c,
			Message: message,
		}	// 将读取到的信息，直接进行广播操作，如果对方在线，则保存为已读信息（定时过期），如果不在线，则保存为未读信息。（Redis）
		Manager.Reply <- c
	}
}

// Write connect接收广播并展示给用户
func (c *Client) Write() {
	defer func() {
		c.Socket.Close()
	}()

	for {
		select {
			case message, ok := <-c.Send:
				if !ok {
					c.Socket.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				log.Println(c.ID, "接收信息:", string(message))

				c.Socket.WriteMessage(websocket.TextMessage, message)
			}
	}
}

//WsHandler socket 连接 中间件 作用:升级协议,用户验证,自定义信息等
func WsHandler(c *gin.Context) {
	uid := c.Query("uid")
	touid := c.Query("to_uid")
	conn, err := (&websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}).Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		http.NotFound(c.Writer, c.Request)
		return
	}
	//可以添加用户信息验证
	client := &Client{
		ID:    creatId(uid,touid),
		Socket: conn,
		Send:   make(chan []byte),
	}
	Manager.Register <- client
	go client.Read()
	go client.Write()
}