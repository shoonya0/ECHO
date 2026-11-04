# WebSocket client example

A small Node client that shows the rules from
[WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md) in code. It needs the `ws` package
(`npm install ws`), because a plain browser page can't send the `Authorization` header.

Three things this example gets right:

- It matches replies using `data.requestId`, which works for every `response`.
- It reads `requestId` at the top level for `error` events.
- It only waits for a reply on requests that get one.

```javascript
import WebSocket from "ws";

export class ChatClient {
  constructor(url, token) {
    this.url = url;
    this.token = token;
    this.lastId = 0;
    this.pending = new Map(); // requestId -> { resolve, reject, timer }
  }

  connect() {
    this.ws = new WebSocket(this.url, {
      headers: { Authorization: `Bearer ${this.token}` },
    });
    this.ws.on("message", (raw) => this.onFrame(JSON.parse(raw)));
    this.ws.on("close", () => console.log("Disconnected"));
    return new Promise((resolve, reject) => {
      this.ws.once("open", resolve);
      this.ws.once("error", reject);
    });
  }

  onFrame(frame) {
    switch (frame.type) {
      case "response":
        return this.finish(frame.data?.requestId, (p) => p.resolve(frame.data));
      case "error":
        return this.finish(frame.requestId, (p) => p.reject(new Error(frame.data.code)));
      case "message":
        return console.log(`${frame.data.senderId}: ${frame.data.content}`);
      default:
        return console.log("Event:", frame.type, frame.data);
    }
  }

  finish(requestId, settle) {
    const entry = this.pending.get(requestId);
    if (!entry) return; // The welcome message and untracked requests end up here.
    clearTimeout(entry.timer);
    this.pending.delete(requestId);
    settle(entry);
  }

  // For requests that get a response: send_message, reactions, invite_user, remove_user.
  request(body) {
    const requestId = `req_${++this.lastId}`;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(requestId);
        reject(new Error("Request timed out"));
      }, 10000);
      this.pending.set(requestId, { resolve, reject, timer });
      this.ws.send(JSON.stringify({ ...body, requestId }));
    });
  }

  // For requests with no response: join_chat, leave_chat, set_typing, mark_read.
  send(body) {
    this.ws.send(JSON.stringify(body));
  }

  sendMessage(chatId, content, messageType = "text") {
    return this.request({ type: "send_message", chatId, content, messageType });
  }

  addReaction(chatId, messageId, emoji) {
    return this.request({ type: "add_reaction", chatId, metadata: { messageId, emoji } });
  }

  openDirectChat(otherUserId) {
    this.send({ type: "join_chat", senderId: otherUserId }); // Watch for the "join" event.
  }

  setTyping(chatId, isTyping) {
    this.send({ type: "set_typing", chatId, metadata: { isTyping } });
  }

  markRead(chatId, messageIds) {
    this.send({ type: "mark_read", chatId, metadata: { messageIds } });
  }
}
```

Using it:

```javascript
const client = new ChatClient("ws://localhost:8080/echo/v1/ws/chat", token);
await client.connect();

const reply = await client.sendMessage(chatId, "Hello!");
console.log("Saved as", reply.messageId);
```

A real app also needs to reconnect after a disconnect, and to load missed messages
[over REST](./rest-api.md#messages) when it does (#43).
