# ZipScope

大量の未解凍 ZIP / CBZ を、展開済みフォルダと同じ感覚で閲覧する Windows 専用のローカル画像ビューアです。
ZIP を解凍せずに中央ディレクトリだけを読み、必要な画像だけをストリーム展開して表示します。
閲覧速度と操作数の少なさに特化しています。

- 対象: Windows 11 x64
- 形式: ZIP / CBZ（Store / Deflate、ZIP64）、JPEG / PNG / GIF（静止画）/ WebP
- ネット接続不要。元の ZIP は読み取り専用で、一切変更しません。

仕様は [docs/SPEC.md](docs/SPEC.md) を参照してください。

## インストール

[GitHub Releases](../../releases) から `ZipScope-<version>-windows-amd64.zip` をダウンロードし、
任意のフォルダに展開して `ZipScope.exe` を実行してください。インストーラは不要です。

WebView2 ランタイムが必要です。Windows 11 には標準で入っています。

## 操作

エクスプローラーと同じ感覚で操作できるよう、フォルダと ZIP を同じ画面で行き来します。
フォルダを開くと ZIP が並び、ZIP を開くと中の画像が並びます。

### 画面構成

```
┌─────────────────────────────────────────────────────┐
│ ← → ↑   📁 C: › Users › you › Pictures › books       │  アドレスバー
├─────────────────────────────────────────────────────┤
│ 📁フォルダ 開く 場所を開く │ 名前変更 移動 削除  未選択 ▤⊞□ 設定 │  コマンドバー
├─────────────────────────────────────────────────────┤
│ 名前                         種類    サイズ   更新日時 │  列ヘッダ（詳細表示）
│ 🖼 book01.zip                12 枚   31 KB  2026-11-21 │
│ 🖼 book02.zip                12 枚   27 KB  2026-11-21 │
├─────────────────────────────────────────────────────┤
│ 3 ZIP / 1 フォルダ · 44 ms            [ZIP を検索]  │  ステータスバー
└─────────────────────────────────────────────────────┘
```

### ナビゲーション

| 操作 | 動作 |
| --- | --- |
| ZIP をダブルクリック / `Enter` | ZIP の中へ入る |
| 画像をダブルクリック / `Enter` | 原寸ビューを開く |
| `Alt+←` / 戻るボタン | 前の場所へ |
| `Alt+→` / 進むボタン | 次の場所へ |
| `Alt+↑` / `Backspace` / 上へボタン | 一つ上の階層へ |
| `F5` / 更新ボタン | 再走査 |
| パンくずの各セグメント | その階層へ直接移動 |
| `Ctrl+O` / 「フォルダ」 | フォルダを選択 |
| ZIP またはフォルダをドロップ | その場所を開く |

### 表示形式

コマンドバー右側のボタンで切り替えます。設定は保存されます。

| 形式 | 内容 |
| --- | --- |
| 詳細 | 名前・種類・サイズ・更新日時の列。列ヘッダで並び替え |
| 中アイコン | サムネイルを並べて表示 |
| 大アイコン | 大きめのサムネイル。スライダーで 64〜256px に調整 |

### 選択

| 操作 | 動作 |
| --- | --- |
| クリック | 1 件選択 |
| `Ctrl+クリック` | 選択に追加 / 解除 |
| `Shift+クリック` | 範囲選択 |
| `↑` / `↓` | 選択を移動 |
| 右クリック | その項目を選択してメニューを開く |

### 原寸ビュー

| 操作 | 動作 |
| --- | --- |
| `←` / `→` | 前 / 次の画像 |
| `Ctrl+←` / `Ctrl+→` | 前 / 次の ZIP |
| `Esc` | 閉じて一覧へ戻る |
| `Home` / `End` | 最初 / 最後の画像 |
| ホイール | カーソル位置を中心にズーム |
| ドラッグ | パン |
| `+` / `-` | ズームイン / アウト |
| `0` | フィット表示に戻す |
| `1` / ダブルクリック | フィット ⇔ 等倍を切り替え |

### 整理（ZIP の移動・名前変更・削除）

ZIP を選んでコマンドバーのボタン、または右クリックメニューから操作します。

| 操作 | 動作 |
| --- | --- |
| 場所を開く | エクスプローラでファイルを選択表示 |
| 名前変更 | その場で名前を変更。拡張子は自動で補完 |
| 移動 | 移動先フォルダを選択して移動 |
| 削除 | 確認後、ごみ箱へ移動（復元可能） |

安全のための制約:

- すべて明示的な操作で、削除は確認ダイアログを挟みます。
- 移動・名前変更の前にファイルハンドルを解放します（Windows のロックを避けるため）。
- 同名ファイルが存在する場合は上書きせず、エラーを表示します。
- 移動先は走査中のルート配下に限定します。
- 名前変更ではパス区切り文字や `..` を拒否します。
- 操作後はキャッシュと一覧を更新します。

### 設定

コマンドバー右端の「設定」から開きます。

- ディスクキャッシュ上限（初期 2 GB）
- メモリキャッシュ上限（初期 256 MiB）
- 先読みワーカー数（初期 4）
- 先読み枚数（初期 2）
- LRU 整理 / サムネイル削除 / すべて消去
- 現在の使用量と先読みの統計

## エクスプローラー統合

`shell/` に COM のインプロセス DLL（`ZipScopeShell.dll`）があり、エクスプローラーに2つの機能を追加します。

| 機能 | 動作 |
| --- | --- |
| サムネイル | ZIP / CBZ のアイコンが、中の最初の画像（自然順先頭）になる |
| 右クリック | 「ZipScopeで開く」でその ZIP を ZipScope で開く |

### インストール

```powershell
# 1. ビルド（MSVC と Windows SDK が必要）
wails build -platform windows/amd64
shell\build.cmd

# 2. インストール（管理者権限は不要）
shell\install.cmd
```

`install.cmd` は以下を行います。

- `ZipScope.exe` と `ZipScopeShell.dll` を `%LOCALAPPDATA%\Programs\ZipScope` に配置
- App Paths を登録（拡張子がビューアを見つけられるようにする）
- COM を登録（`HKCU\Software\Classes` 配下、ユーザー単位）

アンインストールは `shell\uninstall.cmd` を実行します。

サムネイルがすぐに反映されない場合は、サムネイルキャッシュを消してからエクスプローラーを再起動してください。

```powershell
Get-ChildItem "$env:LOCALAPPDATA\Microsoft\Windows\Explorer" -Filter "thumbcache_*.db" | Remove-Item -Force
taskkill /f /im explorer.exe; start explorer.exe
```

### 右クリックメニューの表示場所

Windows 11 の新しい右クリックメニューは、従来の `IContextMenu` ハンドラを直接表示しません。
**「その他のオプションを表示」**（または `Shift+F10`）を開くと「ZipScopeで開く」が表示されます。

新しいメニューに直接出すには `IExplorerCommand` の実装が必要ですが、こちらは Windows 11 専用で
Windows 10 では動作しません。MVP では両方の OS で動く `IContextMenu` を採用しています。

### 安全性

エクスプローラーはこの DLL を `explorer.exe` のプロセス内に読み込みます。そのため:

- すべてのエントリポイントを `try/catch` で囲み、例外を COM 境界の外に出しません
- 展開サイズ・画素数・アーカイブサイズに上限を設けています（アプリ本体より厳しめ）
- 壊れた ZIP や画像以外のエントリは、例外ではなく「サムネイルなし」として扱います
- 右クリックメニューは ZIP / CBZ 以外では自分自身を無効化します

### 検証

`shell/` には4つの検証ツールがあります。いずれも実際の Shell API を通すので、
登録ミスやプロバイダの不具合をエクスプローラーを開く前に検出できます。

```powershell
# ZIP リーダーとデコーダ（DLL と同じソースをリンク）
shell\test.cmd path\to\book.zip

# サムネイルプロバイダ（Shell 経由でサムネイルを取得）
build\shell\thumbtest.exe path\to\book.zip

# 右クリックメニュー（Shell 経由でメニュー項目を列挙）
build\shell\menutest.exe path\to\book.zip

# 右クリックメニューからの起動（実際に ZipScope.exe が起動するか）
build\shell\launchtest.exe path\to\book.zip
```

### ビューアの場所の解決

拡張子は次の順で `ZipScope.exe` を探します。

1. DLL と同じフォルダ（インストール時の配置）
2. 隣の `bin\`（開発中のチェックアウト）
3. `%LOCALAPPDATA%\Programs\ZipScope`
4. App Paths レジストリ（`install.cmd` が登録）
5. `PATH`

見つからない場合はダイアログで通知します（無言で失敗しません）。

## キャッシュと保存場所

すべて `%LOCALAPPDATA%\ZipScope` 配下です。

| パス | 内容 |
| --- | --- |
| `index.db` | SQLite。ZIP のメタデータとエントリ一覧 |
| `thumbs/` | サムネイル画像（2 階層のシャードに分割） |
| `settings.json` | 設定と前回の位置 |

- サムネイルはアーカイブ指紋 + エントリ ordinal / CRC / サイズ + 縮小サイズ + 生成バージョンで識別します。
- ZIP のサイズ・更新日時・中央ディレクトリ指紋が変わると、その ZIP のキャッシュは自動的に無効になります。
- 書き込みは一時ファイルからの原子的な置換です。破損したキャッシュは再生成されます。
- ディスク上限を超えると最終アクセスが古いものから削除されます（LRU）。
- アンインストールは `ZipScope.exe` と `%LOCALAPPDATA%\ZipScope` を削除するだけです。

## 制限（安全側の初期値）

| 制限 | 初期値 | 内容 |
| --- | --- | --- |
| 1 画像の展開サイズ | 128 MiB | ヘッダと実読み込みの両方で検査 |
| 画素数 | 40 MP | デコード前にヘッダで判定 |
| ZIP あたりのエントリ数 | 100,000 | 超過分は読み飛ばして状態表示 |
| メモリキャッシュ | 256 MiB | バイト単位の LRU |
| デコード作業メモリ | 256 MiB | ワーカー横断の重み付きセマフォ |

上限は設定ファイルで引き上げられますが、無制限にはできません。

## 非対応（MVP 外）

RAR / 7z、暗号化 ZIP、入れ子 ZIP、画像編集、クラウド同期。
HTML / SVG などの能動的コンテンツは、拡張子に関わらず表示しません。

ZIP の移動・名前変更・ごみ箱への削除は上記「整理」で対応しています。
完全削除（ごみ箱を経由しない削除）は提供しません。

## 開発

### 必要なもの

- Go 1.26 以上
- Node.js 20 以上
- Wails CLI v2.17 以上（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）
- WebView2 ランタイム

### セットアップ

```powershell
git clone https://github.com/tonbiattack/zip-scope.git
cd zip-scope
cd frontend; npm ci; cd ..
go mod download
```

### 開発実行

```powershell
wails dev
```

フロントエンドの変更はホットリロードされます。

### ビルド

```powershell
wails build -clean -platform windows/amd64
```

成果物は `build/bin/ZipScope.exe` です。

### アイコン

アイコンは `scripts/make-icon.ps1` で生成します。デザインを変えたら再生成してからビルドしてください。

```powershell
pwsh scripts/make-icon.ps1
wails build -platform windows/amd64
```

- `build/appicon.png` — 1024px のマスター画像
- `build/windows/icon.ico` — 16〜256px の 9 サイズを含む ICO

各サイズは縮小ではなくベクターから描き直すため、16px でも潰れません。
エクスプローラー統合の DLL も同じアイコンを使います（`shell/ZipScopeShell.rc`）。

### テスト

```powershell
# Go
go test ./...

# フロントエンド
cd frontend; npm test
```

### ベンチマーク

仕様の性能目標を合成データで測定します。

```powershell
# スモーク（小さいコーパス）
go run ./cmd/zipscope-bench -quick -rounds 5 -out bench-quick.json

# 仕様の測定条件（1000 ZIP × 100 枚）
go run ./cmd/zipscope-bench -archives 1000 -images 100 -width 1600 -height 1200 -rounds 30 -out bench-report.json
```

結果は JSON で出力され、p50 / p95 と目標達成状況が表示されます。
測定値と考察は [docs/PERFORMANCE.md](docs/PERFORMANCE.md) にまとめています。

## 構成

```
main.go                     Wails エントリポイント
internal/app/               アプリケーション層（スキャン・選択・画像配信）
internal/archive/           ZIP 中央ディレクトリ読み取りと画像デコード
internal/scan/              非同期再帰走査
internal/store/             SQLite メタデータ + ディスクサムネイル
internal/thumb/             縮小・エンコード
internal/prefetch/          優先度付きスケジューラ・LRU・重み付きセマフォ
internal/imgserve/          画像 ID の検証と MIME 判定
internal/settings/          設定の永続化
internal/bench/             合成データ生成と統計
cmd/zipscope-bench/         ベンチマークコマンド
shell/                      エクスプローラー統合（C++/COM）
scripts/make-icon.ps1       アイコン生成
frontend/                   Vue 3 + TypeScript
```

## テスト

| 対象 | 件数 | 内容 |
| --- | --- | --- |
| Go | 140+ | ZIP 読み取り、走査、キャッシュ、先読み、画像配信、整理操作、異常系 |
| フロントエンド | 128 | アドレスバー、コマンドバー、ファイル一覧、ビューア、整理 UI、統合 |

異常系は次を検証しています。

- 破損 ZIP、暗号化エントリ、非対応圧縮、切り詰め
- 巨大画像（画素数上限）、ZIP64、日本語・Unicode 名、重複名
- 外付けドライブ切断（ファイル消失）
- キャッシュ失効・破損からの復旧
- 連打しても古い画像が出ないこと
- 元 ZIP が変更されないこと
- 同名上書きの拒否、パス脱出の拒否

## セキュリティ

- 画像は `/img/<id>` という不透明な ID でのみ配信します。ファイルパスや外部 URL は受け付けません。
- ID は正規表現で形状を検証してから解決します。
- 配信する MIME タイプは JPEG / PNG / GIF / WebP のみです。HTML / SVG は返しません。
- 元の ZIP は読み取り専用で開き、ZIP 内のパスをディスクの書き込み先に使いません。
- シンボリックリンクとジャンクションは追跡しません。

## ライセンス

MIT
