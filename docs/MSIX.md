# Microsoft Store 配布の検証結果

ZipScope を MSIX パッケージとして Microsoft Store で配布するための技術検証の記録です。
GitHub Releases での EXE 配布は維持します。

## Partner Center の確定値

2026-11-23 にアプリ名 `ZipScope` の予約が完了し、以下の値が割り当てられました。
これらは `msix/AppxManifest.xml` に反映済みで、`scripts/make-msix.ps1` からも上書きできます。

| 項目 | 値 |
| --- | --- |
| ストア ID | `9PKL9QQXB65R` |
| Package/Identity/Name | `tonbiattack.ZipScope` |
| Package/Identity/Publisher | `CN=2F14A66E-02DF-43E2-93E0-5C5900B38D2E` |
| PublisherDisplayName | `tonbiattack` |
| Package Family Name | `tonbiattack.ZipScope_e28ksxe3fc9w4` |

**Publisher は `CN=tonbiattack` ではありません。** Partner Center が発行する GUID 形式の値で、
一致しないとアップロード時に弾かれます。

Package Family Name は `Identity Name` と Publisher から決まります。
生成したパッケージが正しい PFN を持つことは次のように確認できます。

```powershell
# 生成したマニフェストから PFN を計算し、Partner Center の値と比較する
$name = 'tonbiattack.ZipScope'
$publisher = 'CN=2F14A66E-02DF-43E2-93E0-5C5900B38D2E'
# ... (scripts/verify-pfn.ps1 に実装)
```

## 提出用パッケージの生成

```powershell
# ビルド込みで提出用パッケージを生成
pwsh scripts/make-msix.ps1 -Version 0.1.0.0

# 生成物
#   build/msix/ZipScope_0.1.0.0_x64.msix
```

バージョンは **4 要素**で、**第 4 要素は 0** である必要があります
（Store が改訂番号を付けるため）。スクリプトはこれを検証し、違反時は停止します。

**Store 提出用のパッケージは署名不要です。** Store が再署名します。
`-Sign` はサイドロード検証用の自己署名にのみ使います。

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

## 実機検証の結果（2026-11-23）

サイドロードを試み、**3 つの制約**を特定しました。いずれも環境側の制約で、
パッケージ自体の不備ではありません。

### 1. 署名なしパッケージはインストールできない

```
Add-AppxPackage -Path ZipScope_0.1.0.0_x64.msix
→ 0x800B0100: サブジェクトに署名がありませんでした。
```

MSIX は必ず署名が必要です。`scripts/make-msix.ps1 -Sign` で自己署名できます。

### 2. 自己署名証明書の信頼に管理者権限が必要

```
→ 0x800B0109: 証明書チェーンは処理されましたが、信頼プロバイダーが
   信頼していないルート証明書で強制終了しました。
```

`Cert:\CurrentUser\TrustedPeople` に入れても解決しません。
MSIX の展開は `LocalMachine` の信頼ストアを見るためです。

```
→ 0x80073CFF: このアプリケーションをインストールするには、
   Windows 開発者用ライセンスまたはサイドローディング対応システムが必要です。
```

開発者モードの有効化も `HKLM` への書き込みが必要で、管理者権限が要ります。

### 3. WebView2 の依存宣言が Win32 ランタイムと噛み合わない

`uap10:HostRuntimeDependency` を宣言した状態で登録すると、次のエラーになります。

```
→ 0x80073D31: パッケージでホストのランタイムの依存関係または競合の検証に失敗しました。
```

原因は、この環境の WebView2 が **Win32 ランタイム（EdgeUpdate 経由、v158）** として
インストールされており、MSIX のフレームワークパッケージ（`Microsoft.Win32WebView2`）
としては登録されていないことです。

```
Get-AppxPackage -Name Microsoft.Win32WebView2   # → 何も返らない
Get-AppxPackage | Where-Object Name -like "*WebView*"
# → Microsoft.Win32WebViewHost 10.0.26100.1（これは OS 内蔵の別物）
```

依存宣言を外すとこのエラーは消え、次の制約（開発者モード）に進みました。
つまり**依存宣言自体は正しく解析されている**ことが確認できました。

### 判断: 依存宣言は維持する

Store 経由の配布では、Store がフレームワークパッケージとしての WebView2 を
解決できます。宣言を外すと、WebView2 が入っていない端末で起動時に失敗します。

サイドロードで検証する場合のみ、依存を外したマニフェストを使う選択肢があります。

### 未検証として残る項目

| 項目 | 理由 |
| --- | --- |
| パッケージ版の起動 | 管理者権限（証明書の信頼）が必要 |
| エクスプローラー統合の実動作 | 上記に依存 |
| 更新・アンインストール | 上記に依存 |
| `broadFileSystemAccess` の許可フロー | 上記に依存 |

### 検証手順（管理者権限で実行）

```powershell
# 1. 署名済みパッケージを生成
pwsh scripts/make-msix.ps1 -Version 0.1.0.0 -Sign

# 2. 証明書を信頼する（管理者権限）
Import-Certificate -FilePath build\msix\ZipScope-Test.cer `
  -CertStoreLocation Cert:\LocalMachine\TrustedPeople

# 3. インストール
Add-AppxPackage -Path build\msix\ZipScope_0.1.0.0_x64.msix

# 4. 起動して確認
Start-Process "shell:AppsFolder\tonbiattack.ZipScope_e28ksxe3fc9w4!ZipScope"

# 5. アンインストール
Remove-AppxPackage -Package tonbiattack.ZipScope_0.1.0.0_x64__e28ksxe3fc9w4
```

開発者モードが無効な場合は、設定 → システム → 開発者向け → 開発者モードを ON にします。

## 本人の操作が必要な事項

以下は**代行できません**。

1. **Partner Center の開発者登録**（個人 約$19、組織 約$99。支払いを伴う）
2. **Store への申請・審査提出**
3. **正式なコード署名証明書の取得**（Store 経由なら不要。Store が再署名する）
4. **管理者権限でのサイドロード検証**（上記手順）

## 掲載に必要な資料

| 資料 | 状態 | 場所 |
| --- | --- | --- |
| パッケージ | 生成可 | `scripts/make-msix.ps1` |
| タイル画像一式 | 生成可 | `scripts/make-msix-assets.ps1` → `msix/Assets/` |
| スクリーンショット | **作成済み**（1920×1080、4 枚） | `docs/store/screenshots/` |
| ストア説明文 | **作成済み**（日英） | `docs/store/listing.md` |
| プライバシーポリシー | **公開済み** | https://tonbiattack.github.io/zip-scope/privacy.html |
| サポート URL | GitHub Issues | https://github.com/tonbiattack/zip-scope/issues |
| 年齢評価 | 全年齢 | `docs/store/listing.md` |

掲載文・カテゴリ・キーワードなど、Partner Center の入力内容は
[`docs/store/listing.md`](store/listing.md) にまとめています。

スクリーンショットのサンプルは `cmd/mkscreenshots` で生成します。
図形のみを描画するため、第三者の著作物を含みません。

```powershell
go run ./cmd/mkscreenshots "$env:USERPROFILE\Pictures\ZipScope サンプル"
```

## プライバシー

ZipScope は**ネットワーク通信を行いません**。
閲覧履歴・ファイルパス・画像は外部に送信されません。
ローカルに保存されるのは `%LOCALAPPDATA%\ZipScope` のキャッシュ（SQLite とサムネイル）のみです。

全文は公開済みです: https://tonbiattack.github.io/zip-scope/privacy.html
ソースは [`docs/privacy.md`](privacy.md) で、`.github/workflows/pages.yml` が HTML に変換して公開します。

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

# 2. 提出用パッケージを生成（ビルド込み）
pwsh scripts/make-msix.ps1 -Version 0.1.0.0

# 3. Partner Center の ID と一致するか検証
pwsh scripts/verify-pfn.ps1 -MsixPath build\msix\ZipScope_0.1.0.0_x64.msix

# 4. サイドロード検証用に署名（管理者権限で証明書を信頼してから）
pwsh scripts/make-msix.ps1 -Version 0.1.0.0 -Sign
```

`verify-pfn.ps1` は生成物を展開し、Identity から Package Family Name を
Windows と同じ方法で再計算して Partner Center の値と比較します。
Publisher やバージョンがずれていれば、アップロード前に検出できます。
