# baby-cam 設計仕様書

**バージョン**: 1.0.0  
**作成日**: 2026-03-12  
**ステータス**: Draft

---

## 1. 概要

### 1.1 プロジェクト概要

セルフホスト可能なベビーカメラWebアプリケーション。  
スマートフォン・PCのブラウザだけで動作し、インターネット越しにリアルタイム映像・音声を視聴できる。

### 1.2 設計方針

- **ミニマム実装**: 外部依存を最小限に抑え、総コード量を最小化する
- **セルフホスト優先**: 単一の `docker compose up` で起動できる
- **ブラウザネイティブ**: 専用アプリ不要。ブラウザのWebRTC APIを活用する
- **低遅延**: HLS等ではなくWebRTCによるP2P接続で遅延を最小化する

### 1.3 スコープ外

- 録画・アーカイブ機能
- プッシュ通知
- 複数カメラの同時配信
- モバイルアプリ（iOS/Android）

---

## 2. システムアーキテクチャ

### 2.1 全体構成

```
[インターネット]
       |
 Cloudflare Tunnel  ← ポート開放不要・TLS自動
       |
   nginx コンテナ   ← リバースプロキシ
       |
  Go シグナリングサーバー コンテナ :8080
       |
  静的ファイル配信 (HTML/JS)

[WebRTC メディア経路]
カメラ側ブラウザ ──P2P── ウォッチャー側ブラウザ
      ↕                        ↕
  Cloudflare STUN/TURN    Cloudflare STUN/TURN
```

**メディアデータはサーバーを経由しない。** シグナリング（接続確立のためのメタ情報）のみをサーバーが中継し、映像・音声はブラウザ間でP2P転送される。

### 2.2 コンポーネント一覧

| コンポーネント | 役割 | 技術 |
|---|---|---|
| シグナリングサーバー | WebRTC Offer/Answer/ICEの中継・部屋管理 | Go + gorilla/websocket |
| nginx | HTTPSリバースプロキシ・静的ファイル配信 | nginx:alpine |
| Cloudflare Tunnel | 外部公開・TLS終端 | cloudflared |
| カメラUI | カメラ・マイクアクセス・WebRTC送信側 | HTML/Vanilla JS |
| ウォッチャーUI | WebRTC受信側・映像再生 | HTML/Vanilla JS |

### 2.3 シグナリングシーケンス

```
カメラ              サーバー              ウォッチャー
  |                   |                      |
  |─ join(room,pass) →|                      |
  |← ok / error ──────|                      |
  |                   |← join(room,pass) ────|
  |                   |─ ok / error ────────→|
  |← watcher_joined ──|                      |
  |                   |                      |
  |─ offer(sdp) ─────→|─ offer(sdp) ────────→|
  |←──────────────────|← answer(sdp) ────────|
  |─ ice_candidate ──→|─ ice_candidate ──────→|
  |←──────────────────|← ice_candidate ───────|
  |                   |                      |
  ╔══════════════════════════════════════════╗
  ║  P2P確立。以降、映像・音声はサーバー不介在  ║
  ╚══════════════════════════════════════════╝
  |                   |                      |
  |← watcher_left ────|  (切断時)             |
```

---

## 3. 機能仕様

### 3.1 画面一覧

| 画面 | URL | 説明 |
|---|---|---|
| モード選択 | `/` | カメラ側 / ウォッチャー側を選ぶトップ画面 |
| カメラ画面 | `/camera.html` | 部屋作成・配信開始 |
| ウォッチャー画面 | `/watcher.html` | 部屋一覧・視聴 |

### 3.2 カメラ側の機能

1. 部屋名を入力する
2. パスワードを設定する
3. 配信開始ボタンを押すとカメラ・マイクへのアクセスを要求する
4. サーバーに部屋を登録する（WebSocket接続）
5. ウォッチャーが接続するたびに自動的にP2P接続を確立する
6. 現在の視聴者数を表示する
7. 配信停止ボタンで全接続を切断し部屋を削除する

### 3.3 ウォッチャー側の機能

1. アクティブな部屋の一覧を取得・表示する（`GET /api/rooms`）
2. 部屋を選択してパスワードを入力する
3. サーバーへWebSocket接続し、P2P接続を確立する
4. 映像・音声を再生する
5. ブラウザタブを閉じると自動的に切断する

### 3.4 認証仕様

- **スコープ**: 部屋単位のパスワード認証
- **パスワード伝達**: WebSocket接続時のクエリパラメータ `?pass=xxx`
- **検証タイミング**: WebSocketハンドシェイク時にサーバーが即時検証
- **ハッシュ**: bcryptでサーバー内メモリに保持（永続化なし）
- **失敗時**: WebSocketを `4001 Unauthorized` で即時クローズ

> **注意**: パスワードはHTTPS経由でのみ送信される。HTTP環境での動作は非推奨。

---

## 4. APIインターフェース仕様

### 4.1 REST API

#### `GET /api/rooms`

アクティブな部屋の一覧を返す。

**レスポンス**:
```json
[
  {
    "id": "my-room",
    "watcherCount": 2
  }
]
```

| フィールド | 型 | 説明 |
|---|---|---|
| `id` | string | 部屋ID（表示名兼用） |
| `watcherCount` | number | 現在の視聴者数 |

### 4.2 WebSocket API

#### `WS /ws/camera/:roomID?pass=xxx`

カメラ側の接続エンドポイント。

#### `WS /ws/watch/:roomID?pass=xxx`

ウォッチャー側の接続エンドポイント。

### 4.3 WebSocketメッセージ型

すべてのメッセージはJSON形式。

```typescript
type Message = {
  type: MessageType;
  sdp?:       string;       // offer / answer 時
  candidate?: RTCIceCandidate; // candidate 時
  peerId?:    string;       // 対象ウォッチャーID
  error?:     string;       // error 時
}

type MessageType =
  | "offer"           // カメラ → サーバー → ウォッチャー
  | "answer"          // ウォッチャー → サーバー → カメラ
  | "candidate"       // 双方向
  | "watcher_joined"  // サーバー → カメラ（新規視聴者通知）
  | "watcher_left"    // サーバー → カメラ（視聴者離脱通知）
  | "error"           // サーバー → クライアント
```

---

## 5. データモデル（サーバー内部）

```go
// client.go
type ClientRole int
const (
    RoleCamera  ClientRole = iota
    RoleWatcher
)

type Client struct {
    ID   string          // UUIDv4
    Role ClientRole
    Conn *websocket.Conn
    Send chan []byte      // 送信キュー
}

// room.go
type Room struct {
    ID           string
    PasswordHash string          // bcrypt hash
    Camera       *Client
    Watchers     map[string]*Client  // clientID → Client
    mu           sync.RWMutex
}

// hub.go
type Hub struct {
    Rooms  map[string]*Room  // roomID → Room
    mu     sync.RWMutex
}
```

**状態の永続化なし。** サーバー再起動で全部屋・接続がリセットされる。

---

## 6. ICE / TURN設定

### 6.1 ICEサーバー構成

| 種別 | URL | 用途 |
|---|---|---|
| STUN | `stun:stun.cloudflare.com:3478` | パブリックIP発見 |
| TURN | `turn:turn.cloudflare.com:3478` | NAT越えフォールバック |

### 6.2 Cloudflare TURNクレデンシャル管理

セキュリティのため、**TURNクレデンシャルはフロントエンドに直接埋め込まない。**

```
1. フロントエンドが GET /api/ice-config をリクエスト
2. Goサーバーが Cloudflare API を呼び出し、短命なクレデンシャルを取得
3. サーバーがクレデンシャルをフロントエンドへ返却（有効期限: 1時間）
4. フロントエンドが RTCPeerConnection に設定
```

**環境変数**:
```
CLOUDFLARE_TURN_API_KEY=<Cloudflare Calls TURNサービスのAPIキー>
```

---

## 7. ファイル構成

```
baby-cam/
├── docker-compose.yml
├── .env.example              # 環境変数テンプレート
├── nginx/
│   └── nginx.conf
├── server/
│   ├── Dockerfile
│   ├── go.mod
│   ├── main.go               # エントリーポイント・ルーティング
│   ├── hub.go                # Hub/Room/Client 状態管理
│   └── signaling.go          # WebSocketハンドラ・メッセージルーティング
└── static/
    ├── index.html            # モード選択画面
    ├── camera.html           # カメラ側UI
    └── watcher.html          # ウォッチャー側UI
```

### 7.1 実装ボリューム見積もり

| ファイル | 見積行数 | 備考 |
|---|---|---|
| `main.go` | ~120行 | HTTPサーバー・ルーティング・ICE API |
| `hub.go` | ~100行 | 状態管理・ロック制御 |
| `signaling.go` | ~80行 | メッセージデシリアライズ・転送 |
| `index.html` | ~60行 | ボタン2つのシンプルな画面 |
| `camera.html` | ~130行 | getUserMedia・WebRTC送信側 |
| `watcher.html` | ~130行 | 部屋一覧・WebRTC受信側 |
| `nginx.conf` | ~25行 | |
| `docker-compose.yml` | ~45行 | |
| **合計** | **~690行** | |

---

## 8. デプロイ構成

### 8.1 docker-compose.yml 概要

```yaml
services:
  app:
    build: ./server
    environment:
      - CLOUDFLARE_TURN_API_KEY
    expose:
      - "8080"

  nginx:
    image: nginx:alpine
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./static:/usr/share/nginx/html:ro
    depends_on:
      - app

  cloudflared:
    image: cloudflare/cloudflared:latest
    command: tunnel --no-autoupdate run
    environment:
      - TUNNEL_TOKEN
    depends_on:
      - nginx
```

### 8.2 環境変数一覧

| 変数名 | 必須 | 説明 |
|---|---|---|
| `TUNNEL_TOKEN` | ✅ | Cloudflare TunnelのトークンをCloudflareダッシュボードで発行 |
| `CLOUDFLARE_TURN_API_KEY` | ✅ | Cloudflare Calls TURNのAPIキー |

### 8.3 起動手順

```bash
# 1. リポジトリをクローン
git clone https://github.com/yourname/baby-cam
cd baby-cam

# 2. 環境変数を設定
cp .env.example .env
vi .env  # TUNNEL_TOKEN, CLOUDFLARE_TURN_API_KEY を記入

# 3. 起動
docker compose up -d

# 4. Cloudflareダッシュボードで発行されたURLにアクセス
#    例: https://baby-cam.yourdomain.com
```

---

## 9. セキュリティ考慮事項

| 項目 | 対策 |
|---|---|
| 通信経路の暗号化 | Cloudflare Tunnel経由のHTTPS必須 |
| 不正アクセス | 部屋ごとのbcryptパスワード認証 |
| TURNクレデンシャル漏洩 | サーバー側で動的生成・クライアントに直接埋め込まない |
| DoS（部屋の無制限作成） | 将来的にレートリミット追加（初期実装はスコープ外） |
| カメラアクセス | ブラウザがHTTPS環境でのみ許可する仕様に依存 |

---

## 10. 制約・既知の制限

- サーバー再起動時にすべての部屋・接続がリセットされる（インメモリ状態のみ）
- 同時接続ウォッチャー数は2〜5人を想定。それ以上はカメラ側のアップリンク帯域がボトルネックになる（カメラが各ウォッチャーへ個別にP2P送信するため）
- モバイルキャリア回線など対称型NATではTURNが必須となる場合がある
- 録画・通知機能は本バージョンのスコープ外

---

## 11. 今後の拡張候補

| 機能 | 概要 |
|---|---|
| 録画 | サーバーサイドでMediaRecorder APIを使ってファイル保存 |
| 動体検知通知 | カメラ側JSで動体検知 → WebhookでPush通知 |
| SFU導入 | 視聴者数が増えた場合に mediasoup 等を検討 |
| 複数部屋対応 | 現行設計でも対応済み（Hub構造） |

