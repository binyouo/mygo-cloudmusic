# WebSocket

WebSocket 插件为页面提供一个 `WebSocket`，其连接由 Go 建立，API 与浏览器的相同。与 webview 自带的 WebSocket 不同，它：

- 可以在建立连接的握手中发送任意请求头，例如 `Authorization`、`Cookie` 或 `Origin`，适用于要求在请求头中携带令牌的服务器；
- 不受页面的内容安全策略约束；
- 服务器发送消息的速度超过页面处理速度时，将待处理消息留在 Go 中，而不是放在页面内存中。

## 设置

在 Go 中使用插件：

```go
import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/websocket"
)

func main() {
	mygo.Use(websocket.Plugin)
	// ...
}
```

并把它的包添加到前端：

```sh
bun add @mygo-plugins/websocket
```

在不使用 Go 部分的应用中打开的连接会失败，控制台会提示调用 `mygo.Use`。

## 连接

从包中导入 `WebSocket`，像浏览器的 `WebSocket` 一样使用它：

```ts
import { WebSocket } from "@mygo-plugins/websocket";

const ws = new WebSocket("wss://example.com/live", ["v1"], {
  headers: { authorization: `Bearer ${token}` },
});
ws.binaryType = "arraybuffer";
ws.onopen = () => ws.send(JSON.stringify({ subscribe: "prices" }));
ws.onmessage = (e) => console.log(e.data);
ws.onclose = (e) => console.log(e.code, e.reason, e.wasClean);
```

前两个参数与浏览器 `WebSocket` 相同：使用 `ws:` 或 `wss:` 协议的 URL（`http:` 和 `https:` 会分别转换为这两种协议，相对 URL 以页面 URL 为基准解析），以及子协议列表，服务器从中选择一个，连接建立后可通过 `ws.protocol` 读取。第三个参数是插件扩展的 `headers`，用于指定建立连接时的握手请求头，可以是 `Headers`、对象或键值对。

其余都是浏览器的 API：`readyState` 及其常量，`send`（可发送字符串、`ArrayBuffer`、类型化数组和 `Blob`），`close(code, reason)`，`binaryType`（默认 `"blob"`，或 `"arraybuffer"`），以及 `open`、`message`、`error` 和 `close` 事件，可通过 `on…` 属性或 `addEventListener` 使用。

有些库接受一个 `WebSocket` 类，并用两个参数创建连接，GraphQL 和实时客户端经常如此；可以给它们一个会添加请求头的类：

```ts
class AuthWebSocket extends WebSocket {
  constructor(url: string | URL, protocols?: string | string[]) {
    super(url, protocols, { headers: { authorization: `Bearer ${token}` } });
  }
}
```

## 发送与接收

- 连接尚未建立时，`send` 会抛出异常；连接关闭后发送的消息会被丢弃，与浏览器的行为相同。消息按发送顺序发出。
- `bufferedAmount` 统计传给 `send` 但 Go 尚未写出的字节：发送很多内容的页面可以等它下降。
- 当页面还有超过 1 MiB 的消息未处理时，来自服务器的消息会留在 Go 中等待，因此比页面更快的服务器无法填满页面的内存。大于 `MaxMessageSize`（默认 64 MiB）的消息会使连接失败。
- Go 会回应服务器的 ping。

## 关闭

`close(code, reason)` 接受 1000 或 3000 到 4999 之间的代码，以及最多 123 字节的原因，如同浏览器的 `close`。随后 Go 会等待服务器完成关闭握手：服务器完成时，`close` 事件带有服务器的代码和原因，`wasClean` 为 true；在 `CloseTimeout`（默认 5 秒）后仍未完成时，Go 断开连接，事件的代码为 1006。

当连接所在的页面导航离开或其窗口关闭时，连接会自行关闭，告诉服务器它要离开（代码 1001）。

连接失败时，例如无法连接服务器、服务器响应握手但未升级连接、返回重定向或违反协议，页面会先收到 `error` 事件，再收到代码为 1006 的 `close` 事件，控制台会说明原因，与浏览器的行为相同。

## 与浏览器的 WebSocket 的差异

- 构造函数接受第三个参数，即握手的请求头。
- 页面的内容安全策略（`connect-src`）不适用，webview 的 cookie 也不适用：Go 一侧 `http.Client` 的 cookie 适用（见[选项](#选项)）。
- 不协商任何扩展，例如 permessage-deflate：`extensions` 始终为空。

## 选项

`websocket.Plugin` 通过系统的代理设置打开到任何 URL 的连接（见[代理](../native.md#代理)）。`websocket.New` 改为接受选项：

```go
mygo.Use(websocket.New(websocket.Options{
	Client: &http.Client{Transport: transport},
	Allow: func(r *http.Request) bool {
		return r.URL.Hostname() == "live.example.com"
	},
	MaxMessageSize: 16 << 20,        // 16 MiB; default 64 MiB
	CloseTimeout:   2 * time.Second, // default 5 seconds
}))
```

- `Client` 执行建立连接的握手，因此会使用其 `Transport`（代理、TLS 设置）和 `Jar`（cookie）。`Timeout` 和 `CheckRedirect` 会被忽略：WebSocket 是长连接，重定向会使连接失败，与浏览器的行为相同。
- `Allow` 接收每个连接的握手请求，包括 URL 和请求头，并决定是否允许页面建立该连接；拒绝时连接会失败。
- `MaxMessageSize` 是来自服务器的消息可以有的最大字节数；更大的消息会使连接失败。
- `CloseTimeout` 是页面主动关闭的连接等待服务器完成关闭握手的时长。

只有应用自己的页面可以调用插件，如同每个绑定方法（见[谁可以调用](../bindings.md#谁能调用)）。
