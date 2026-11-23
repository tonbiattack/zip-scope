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
