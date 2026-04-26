### `POST /anthropic/v1/messages`

**请求头**：

```http
x-api-key: your-api-key
Content-Type: application/json
anthropic-version: 2023-06-01
```

> `anthropic-version` 可省略，服务端会自动补为 `2023-10-01`。 // Updated version