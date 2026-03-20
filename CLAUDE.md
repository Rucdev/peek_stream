# CLAUDE.md

## プロジェクト概要

**peek_stream**（仕様書内では baby-cam）— セルフホスト可能なベビーカメラWebアプリ。
ブラウザのみで動作し、WebRTCによるP2P接続でリアルタイム映像・音声を配信する。

## アーキテクチャ

```
[Internet]
    |
ngrok Tunnel (TLS・ポート開放不要)
    |
nginx コンテナ (リバースプロキシ + 静的ファイル配信)
    |
Go シグナリングサーバー :8080

[メディア経路] カメラブラウザ ──P2P── ウォッチャーブラウザ
               (映像・音声はサーバーを経由しない)
```

## ファイル構成

```
peek_stream/
├── CLAUDE.md
├── docker-compose.yml         # 本番: app + nginx + cloudflared
├── docker-compose.dev.yml     # 開発用
├── .env.example               # 環境変数テンプレート
├── nginx/nginx.conf
├── server/
│   ├── Dockerfile
│   ├── go.mod / go.sum
│   ├── main.go       # HTTPサーバー・ルーティング・ICE API
│   ├── hub.go        # Hub/Room/Client 状態管理（インメモリ）
│   └── signaling.go  # WebSocketハンドラ・メッセージルーティング
└── static/
    ├── index.html    # モード選択画面
    ├── camera.html   # カメラ側UI（getUserMedia・WebRTC送信）
    └── watcher.html  # ウォッチャー側UI（部屋一覧・WebRTC受信）
```

## API

### REST
- `GET /api/rooms` — アクティブな部屋一覧 `[{id, watcherCount}]`
- `GET /api/ice-config` — ICE設定（STUN/TURNクレデンシャル）

### WebSocket
- `WS /ws/camera/:roomID?pass=xxx` — カメラ側接続
- `WS /ws/watch/:roomID?pass=xxx` — ウォッチャー側接続

### WebSocketメッセージ型
`offer` / `answer` / `candidate` / `watcher_joined` / `watcher_left` / `error`

## 環境変数

| 変数 | 必須 | 説明 |
|------|------|------|
| `NGROK_AUTHTOKEN` | ✅ | ngrok認証トークン |

## 技術スタック

- **バックエンド**: Go + `gorilla/websocket` + `golang.org/x/crypto/bcrypt`
- **フロントエンド**: Vanilla HTML/JS（フレームワークなし）
- **認証**: 部屋単位のbcryptパスワード（インメモリ、永続化なし）
- **ICE**: Google STUN（`stun.l.google.com:19302`）
- **トンネル**: ngrok（`NGROK_AUTHTOKEN`で認証）

## 設計方針

- **ミニマム実装**: 外部依存最小・総コード量最小
- **単一コマンド起動**: `docker compose up -d`
- **状態永続化なし**: サーバー再起動で全部屋・接続リセット
- スコープ外: 録画、プッシュ通知、複数カメラ同時配信、モバイルアプリ

## 開発・起動

```bash
# 開発
docker compose -f docker-compose.dev.yml up

# 本番
cp .env.example .env
# .env に NGROK_AUTHTOKEN を記入（https://dashboard.ngrok.com/get-started/your-authtoken）
docker compose up -d

# ngrokが払い出したURLを確認
docker compose logs ngrok
```

## 未実装・TODO

- `handleICEConfig`: 現在はGoogle STUNのみ返す。NAT越えが困難な環境ではTURNサーバーの追加を検討。
