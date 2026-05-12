<p align="center">
  <img src="webui/public/ds2api-favicon.svg" width="128" height="128" alt="DS2API icon" />
</p>

# DS2API

Chuyển đổi khả năng trò chuyện Web của DeepSeek thành các API tương thích với OpenAI, Claude và Gemini. Backend được xây dựng bằng Go, kết hợp với một cầu nối Node Runtime nhỏ cho việc streaming trên Vercel, và bảng điều khiển quản trị React WebUI nằm trong thư mục `webui/`.

Ngôn ngữ: [中文](README.MD) | [English](README.en.md) | [Tiếng Việt](README.vi.md)

## Các tính năng chính

| Tính năng | Chi tiết |
| --- | --- |
| Tương thích OpenAI | Hỗ trợ đầy đủ các endpoint `/v1/chat/completions`, `/v1/responses`, `/v1/embeddings`, `/v1/files`, v.v. |
| Tương thích Claude | Hỗ trợ các endpoint `/anthropic/v1/messages` và các đường dẫn tắt tương ứng. |
| Tương thích Gemini | Hỗ trợ `generateContent` và `streamGenerateContent`. |
| Xoay vòng nhiều tài khoản | Tự động làm mới token, hỗ trợ đăng nhập bằng Email và Số điện thoại. |
| Kiểm soát truy cập | Giới hạn số yêu cầu đồng thời trên mỗi tài khoản và hàng đợi thông minh. |
| Giải mã DeepSeek PoW | Bộ giải mã hiệu suất cao viết bằng Go thuần, phản hồi trong mili giây. |
| Hỗ trợ Tool Calling | Xử lý chống rò rỉ, hỗ trợ gọi công cụ có cấu trúc. |
| Bảng điều khiển Quản trị | Giao diện web hiện đại tại `/admin` (Hỗ trợ đa ngôn ngữ Trung/Anh/Việt, chế độ tối). |

## Khởi động nhanh

### Cách triển khai khuyến nghị:

1. **Tải về bản build sẵn**: Cách dễ nhất cho hầu hết người dùng.
2. **Triển khai Docker**: Phù hợp cho môi trường container.
3. **Triển khai Vercel**: Phù hợp nếu bạn muốn dùng serverless.
4. **Chạy từ mã nguồn**: Dành cho nhà phát triển muốn tùy chỉnh.

### Bước chuẩn bị chung:

Sử dụng `config.json` làm nguồn cấu hình chính:

```bash
cp config.example.json config.json
# Chỉnh sửa config.json với thông tin tài khoản DeepSeek của bạn
```

### Chạy cục bộ:

**Yêu cầu**: Go 1.26+, Node.js 20.19+ (nếu build WebUI cục bộ).

**Windows — khởi động 1 click**: double-click `start.bat` ở thư mục gốc repo. Script tự động cài Go 1.26.3 nếu chưa có, tự sao chép `config.example.json` thành `config.json` lần đầu chạy (và mở bằng Notepad để điền thông tin), đọc `PORT` từ `.env`, rồi chạy `go run ./cmd/ds2api` — không cần cấu hình thủ công.

```bash
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api
cp config.example.json config.json
go run ./cmd/ds2api
```

URL mặc định: `http://127.0.0.1:5001`

## Tài liệu

| Tài liệu | Mô tả |
| --- | --- |
| [API.en.md](API.en.md) | Tài liệu tham khảo API với các ví dụ |
| [DEPLOY.en.md](docs/DEPLOY.en.md) | Hướng dẫn triển khai chi tiết |

## Miễn trừ trách nhiệm

Dự án này được xây dựng thông qua kỹ thuật đảo ngược (reverse engineering) và chỉ được cung cấp cho mục đích học tập, nghiên cứu và thử nghiệm cá nhân. Không có ủy quyền thương mại nào được cấp, và không có đảm bảo về tính ổn định hay kết quả sử dụng. Tác giả không chịu trách nhiệm cho bất kỳ tổn thất hoặc rủi ro pháp lý nào phát sinh từ việc sử dụng dự án này.
