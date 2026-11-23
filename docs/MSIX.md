# Microsoft Store 配布の検証結果

ZipScope を MSIX パッケージとして Microsoft Store で配布するための技術検証の記録です。
GitHub Releases での EXE 配布は維持します。

## 結論

**MSIX 化は可能です。** ビューア本体・エクスプローラー統合の両方がパッケージ方式で動作する見込みで、
`build/msix/ZipScope_<version>_x64.msix` を生成できるところまで確認しました。

ただし、**Store への申請と Partner Center 登録は本人の操作が必要**です（後述）。

## 検証済みの事項

| 項目 | 結果 |
| --- | --- |
| MSIX ツール | Windows SDK 10.0.26100 の `makeappx.exe` / `signtool.exe` で生成・署名できる |
| パッケージ生成 | 成功（6.75 MB、12 ファイル） |
| 署名 | 自己署名証明書で成功 |
| マニフェスト検証 | `makeappx pack` のスキーマ検証を通過 |
| 展開検証 | `makeappx unpack` で全ファイルが揃うことを確認 |
| サムネイルハンドラ | `desktop2:ThumbnailHandler` で宣言可能 |
| 右クリックメニュー | `desktop9:FileExplorerClassicContextMenuHandler` で宣言可能 |
| WebView2 | `uap10:HostRuntimeDependency` で依存を宣言可能 |
| フルトラスト | `rescap:runFullTrust` + `rescap:broadFileSystemAccess` |

## アーキテクチャ上の判断

### フルトラスト・デスクトップパッケージとして扱う

Wails アプリは Win32 プロセスで、UWP サンドボックス内では動きません。
`EntryPoint="Windows.FullTrustApplication"` を指定し、`runFullTrust` を要求します。

### COM はレジストリではなくマニフェストで登録する

パッケージアプリは `HKEY_CLASSES_ROOT` に書き込めません。
`com:SurrogateServer` で CLSID を宣言し、`desktop2:ThumbnailHandler` /
`desktop9:FileExplorerClassicContextMenuHandler` から参照します。

**DLL 自体は変更不要**でした。既存の `IThumbnailProvider` / `IContextMenu` 実装をそのまま使えます。

### 右クリックメニューは従来方式（IContextMenu）を採用

新しい Windows 11 メニューに直接出すには `IExplorerCommand` が必要ですが、以下を理由に採用しませんでした。

- Windows 10 では動作しない（`desktop4:FileExplorerContextMenus` は Windows 11 21H2 以降）
- 既存の `IContextMenu` 実装を二重に持つことになる

そのため Store 版でも「その他のオプションを表示」（`Shift+F10`）に表示されます。
GitHub Releases 版と同じ挙動です。

### ファイルアクセス

`broadFileSystemAccess` を要求します。ZIP は外付けドライブや任意のフォルダにあるため、
ライブラリ限定のアクセスでは足りません。

この capability は**ユーザーが設定画面で許可する**必要があります（初回起動時）。
拒否された場合、アプリはフォルダ選択ダイアログ経由でのみアクセスできます。

## 未検証の事項

以下は実機での確認が必要です。この環境では**管理者権限が無く**、
証明書を `LocalMachine\TrustedPeople` に置けないためサイドロードできませんでした。

| 項目 | 理由 |
| --- | --- |
| サイドロードでの起動 | 自己署名証明書の信頼に管理者権限が必要 |
| パッケージ版でのエクスプローラー統合の実動作 | 上記に依存 |
| 更新・アンインストール | 上記に依存 |
| 外付けドライブのアクセス許可フロー | 上記に依存 |

検証手順は `scripts/make-msix.ps1` の出力に記載しています。

```powershell
# 管理者権限で実行
Import-Certificate -FilePath build\msix\ZipScope-Test.cer `
  -CertStoreLocation Cert:\LocalMachine\TrustedPeople
Add-AppxPackage -Path build\msix\ZipScope_0.1.0.0_x64.msix
```

## 本人の操作が必要な事項

以下は**代行できません**。

1. **Partner Center の開発者登録**（個人 約$19、組織 約$99。支払いを伴う）
2. **アプリ名 `ZipScope` の予約**（先着順。既に取得されている場合は別名が必要）
3. **Store への申請・審査提出**
4. **正式なコード署名証明書の取得**（Store 経由なら不要。Store が再署名する）

## 掲載に必要な資料

| 資料 | 状態 |
| --- | --- |
| パッケージ | 生成可（`scripts/make-msix.ps1`） |
| タイル画像一式 | 生成可（`scripts/make-msix-assets.ps1`） |
| スクリーンショット | **未作成**（実機で撮影が必要） |
| ストア説明文 | **未作成**（下書きは下記） |
| プライバシーポリシー | **未作成**（ネット通信なしの旨を明記） |
| サポート URL | GitHub Issues を利用可 |
| 年齢評価 | 全年齢（画像閲覧のみ） |

### 説明文の下書き

> **ZipScope — ZIP の中を、フォルダのように見る**
>
> ZIP や CBZ を解凍せずに、中の画像をそのまま閲覧できる Windows 用のビューアです。
> 大量のアーカイブを扱うときも、サムネイルが素早く並び、クリックだけで読めます。
>
> ・ZIP を展開しない — 中央ディレクトリだけを読み、必要な画像だけを取り出します
> ・エクスプローラー統合 — ZIP のアイコンが表紙になり、右クリックから直接開けます
> ・1万件でも軽快 — 画面内だけを描画する仮想スクロール
> ・オフラインで完結 — ネット接続は一切行いません
>
> 対応形式: ZIP / CBZ（Store / Deflate、ZIP64）、JPEG / PNG / GIF / WebP

## プライバシー

ZipScope は**ネットワーク通信を行いません**。
閲覧履歴・ファイルパス・画像は外部に送信されません。
ローカルに保存されるのは `%LOCALAPPDATA%\ZipScope` のキャッシュ（SQLite とサムネイル）のみです。

## 配布方式の比較

| 方式 | 長所 | 短所 |
| --- | --- | --- |
| **GitHub Releases（現行）** | 即時配布、審査不要、証明書不要 | SmartScreen 警告、手動更新 |
| **MSIX（Store）** | 自動更新、SmartScreen 警告なし、発見性 | 審査、開発者登録、`broadFileSystemAccess` の許可が必要 |
| MSIX（サイドロード） | 自動更新なしで上記の長所 | 証明書の信頼設定が必要 |

**両方を維持する方針**とします。Store 版は「見つけてもらいやすく更新が楽」、
Releases 版は「すぐ試せる・審査を待たない」という役割分担です。

## 再現方法

```powershell
# 1. アイコンとタイル画像を生成
pwsh scripts/make-icon.ps1
pwsh scripts/make-msix-assets.ps1

# 2. パッケージを生成（ビルド込み）
pwsh scripts/make-msix.ps1 -Version 0.1.0.0

# 3. 署名してローカル検証
pwsh scripts/make-msix.ps1 -Version 0.1.0.0 -Sign
```
