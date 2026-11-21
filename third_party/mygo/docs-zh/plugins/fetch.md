# Fetch

fetch 插件为页面提供一个 `fetch`，由 Go 发起 HTTP 请求。它接受标准 `fetch` 的选项，返回标准的 `Response`，但请求从应用的 Go 一侧发出，因此：

- 它不受 CORS 限制：可向任意服务器发起请求，除 `Set-Cookie` 和 `Set-Cookie2`（插件会过滤它们）外，每个响应头都可读取；
- 它可以设置任何请求头，包括 webview 禁止的，例如 `Origin`、`Cookie`、`Referer` 和 `User-Agent`；
- 响应正文随 Go 的读取流式传给页面，中止请求也会取消 Go 端的请求。

## 设置

在 Go 中使用插件：

```go
import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/fetch"
)

func main() {
	mygo.Use(fetch.Plugin)
	// ...
}
```

并把它的包加到前端：

```sh
bun add @mygo-plugins/fetch
```

在不使用 Go 部分的应用中调用它的 `fetch`，会被拒绝，错误会提示调用 `mygo.Use`。

## 发起请求

从包中导入 `fetch`，像使用 webview 的 `fetch` 那样调用它：

```ts
import { fetch } from "@mygo-plugins/fetch";

const res = await fetch("https://api.example.com/items", {
  method: "POST",
  headers: {
    authorization: `Bearer ${token}`,
    "content-type": "application/json",
  },
  body: JSON.stringify({ name: "Ada" }),
});
if (!res.ok) throw new Error(`HTTP ${res.status}`);
const item = await res.json();
```

`Request` 可以代替 URL 使用。请求正文与标准 `fetch` 的一样，例如字符串、`FormData`、`URLSearchParams`、`Blob` 和 `ArrayBuffer`，并带上标准 `fetch` 会给它们的 `Content-Type`；请求开始前会完整读取它们，流也一样。

许多 API 客户端之类的库允许传入自己的 `fetch`，可以把插件的实现传给它们。不要把它赋给 `globalThis.fetch`：插件会把不由它发起请求的 URL（协议不是 http 或 https 的）交给 webview 的 `fetch`。

## 流式响应

响应正文是一个 `ReadableStream`，Go 收到服务器发来的数据后就写入流中，适合服务端推送事件（SSE）和模型 API 的流式回复：

```ts
const res = await fetch("https://api.example.com/events");
const reader = res.body!.pipeThrough(new TextDecoderStream()).getReader();
for (;;) {
  const { value, done } = await reader.read();
  if (done) break;
  output.append(value);
}
```

Go 的读取不会快过页面：当页面还有超过 1 MiB 的正文未读取时，它会等待，因此大文件下载不会堆积在页面的内存中。

## 取消

中止请求的 `signal` 会在 Go 中取消它，无论响应是否已经到达：

```ts
const ctrl = new AbortController();
const timer = setTimeout(() => ctrl.abort(), 10_000);
try {
  const res = await fetch(url, { signal: ctrl.signal });
  return await res.text();
} finally {
  clearTimeout(timer);
}
```

promise 或正文的读取会以该 `signal` 的 `reason` 拒绝。取消正文（`res.body.cancel()`）、导航到别处和关闭窗口也会取消请求。

## 错误

与 webview 的 `fetch` 一样，HTTP 错误状态码也不会使请求失败：检查 `res.ok`。请求失败时，promise 会以 `TypeError` 拒绝，例如无法连接服务器、TLS 失败或重定向过多，错误消息会说明原因：

```ts
try {
  await fetch("https://unreachable.example");
} catch (err) {
  console.log(err.message); // fetch failed: Get "https://unreachable.example": dial tcp: lookup unreachable.example: no such host
}
```

## 与 webview 的 fetch 的区别

- `mode`、`credentials`、`cache`、`referrerPolicy`、`integrity`、`keepalive` 和 `priority` 不适用，会被忽略。
- Cookie 属于 Go 一侧的 `http.Client`，绝不是 webview 的：除非客户端有 `Jar` 保存它们（见[选项](#选项)），否则没有 Cookie。
- `redirect: "manual"` 返回重定向响应本身，带其状态和 `Location`，而不是不透明响应。`"error"` 会在遇到重定向时拒绝，默认的 `"follow"` 最多跟随 20 次重定向，并设置 `res.redirected` 和 `res.url`。
- 协议不是 http 或 https 的 URL，例如构建版本中应用自己的页面（`mygo://localhost`）、`data:` 和 `blob:` URL，交给 webview 的 `fetch`。相对 URL 按页面的 URL 解析：在 `mygo dev` 期间，它们是开发服务器的 URL，由 Go 获取。

## 选项

`fetch.Plugin` 通过系统的代理设置发起请求，与 webview 一样（见[代理](../native.md#代理)），并允许页面请求任何 URL。`fetch.New` 则接受选项：

```go
jar, _ := cookiejar.New(nil)

mygo.Use(fetch.New(fetch.Options{
	Client: &http.Client{Jar: jar, Timeout: time.Minute},
	Allow: func(r *http.Request) bool {
		return r.URL.Hostname() == "api.example.com"
	},
}))
```

- `Client` 负责发起请求，因此它的 `Transport`（代理、TLS 设置）、`Jar`（请求之间保存的 Cookie）和 `Timeout` 都会生效，它的 `CheckRedirect` 限制跟随的重定向。没有提供时，传输层使用 `netproxy.Transport()`：环境变量（`HTTPS_PROXY`、`NO_PROXY`）指定了代理时用环境变量的代理，否则用系统的。自己的客户端会保留自己的 `Transport`；要使用同样的代理，请给它 `netproxy.Transport()`，或把它的 `Proxy` 设为 `netproxy.Proxy`。
- `Allow` 在 Go 发起请求前接收请求的 URL、方法和请求头，并决定是否允许页面发起该请求；不允许时，请求会以错误拒绝。它只检查初始请求，不检查随后的重定向；要限制重定向，使用客户端的 `CheckRedirect`。

只有应用自己的页面可以调用插件，与每个绑定方法一样（见[谁可以调用](../bindings.md#谁能调用)）；`Allow` 可以阻止运行了不该运行的代码的页面（例如通过跨站脚本漏洞）访问其他服务器。
