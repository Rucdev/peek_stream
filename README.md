# PeakStream 📷

> Self-hosted baby monitor over the internet — no app required.

ブラウザだけで動くセルフホスト型ライブカメラアプリ。  
スマートフォン・PCのカメラ映像をインターネット越しにリアルタイム視聴できます。

## Features

- **ブラウザだけで完結** — iOS/Android アプリ不要。Safari/Chrome で動作
- **低遅延** — HLS ではなく WebRTC P2P 接続。遅延は通常 < 1秒
- **インターネット越し対応** — Cloudflare Tunnel によりポート開放・証明書設定不要
- **部屋ごとパスワード** — 知っている人だけが視聴できる
- **複数人同時視聴** — 2〜5人の同時接続を想定
- **`docker compose up` 一発起動** — セットアップ最小限

## How it works

```
[カメラ側ブラウザ]
       |
       | WebSocket (シグナリングのみ)
       ↓
[Go シグナリングサーバー] ── [Cloudflare Tunnel] ── インターネット
       ↑
       | WebSocket (シグナリングのみ)
[ウォッチャー側ブラウザ]

映像・音声は WebRTC P2P で直接転送 (サーバーを経由しない)
```

## Requirements

- Docker & Docker Compose
- Cloudflare アカウント（無料）
  - [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) トークン
  - [Cloudflare Calls TURN](https://developers.cloudflare.com/calls/turn/) API キー

## Quick Start

```bash
# 1. クローン
git clone https://github.com/yourname/peakstream
cd peakstream

# 2. 環境変数を設定
cp .env.example .env
vi .env
```

`.env` に以下を記入します：

```env
TUNNEL_TOKEN=<Cloudflare Tunnel のトークン>
CLOUDFLARE_TURN_API_KEY=<Cloudflare Calls TURN の API キー>
```

```bash
# 3. 起動
docker compose up -d

# 4. Cloudflare ダッシュボードに表示された URL にアクセス
#    例: https://peakstream.yourdomain.com
```

## Usage

### カメラ側（配信する）

1. トップ画面で **「カメラ側」** を選択
2. 部屋名とパスワードを入力して **「配信開始」**
3. ブラウザのカメラ・マイクアクセスを許可
4. 配信中は視聴者数がリアルタイムで表示される

### ウォッチャー側（視聴する）

1. トップ画面で **「ウォッチャー側」** を選択
2. アクティブな部屋の一覧から見たい部屋を選択
3. パスワードを入力して **「視聴開始」**

## Architecture

| コンポーネント | 技術 | 役割 |
|---|---|---|
| シグナリングサーバー | Go + gorilla/websocket | WebRTC Offer/Answer/ICE の中継 |
| リバースプロキシ | nginx:alpine | 静的ファイル配信 |
| 外部公開 | Cloudflare Tunnel | HTTPS・ポート開放不要 |
| NAT 越え | Cloudflare STUN/TURN | P2P 接続のフォールバック |
| フロントエンド | Vanilla JS | アプリ不要・ブラウザネイティブ |

## Repository Structure

```
peakstream/
├── docker-compose.yml
├── .env.example
├── nginx/
│   └── nginx.conf
├── server/
│   ├── Dockerfile
│   ├── go.mod
│   ├── main.go         # ルーティング・ICE config API
│   ├── hub.go          # 部屋・接続の状態管理
│   └── signaling.go    # WebSocket メッセージ中継
└── static/
    ├── index.html      # モード選択
    ├── camera.html     # カメラ側 UI
    └── watcher.html    # ウォッチャー側 UI
```

## Limitations

- サーバー再起動時にすべての部屋・接続がリセットされます（インメモリ）
- 同時視聴者数が増えるとカメラ側のアップリンク帯域がボトルネックになります（P2P 1:N のため）
- 録画・プッシュ通知機能は未実装です

## Roadmap

- [ ] 動体検知 → Webhook 通知
- [ ] 録画・クリップ保存
- [ ] SFU 導入（大人数視聴対応）

## License

MIT
