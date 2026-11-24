# 認定差し戻し 10.3.4 の原因と修正

2026-11-24 に Microsoft から「The product failed to install through the Store.」
（認定項目 10.3.4 App Is Testable）で差し戻されました。
エラーコードの記載はなく、機能テストに到達していませんでした。

## 結論

**マニフェストに 3 つの誤りがありました。** いずれも `makeappx` のスキーマ検証を通過し、
Partner Center の受領検証も通るため、提出時には検出できませんでした。

| # | 誤り | 影響 |
| --- | --- | --- |
| 1 | WebView2 の依存宣言が不正 | インストール時に依存解決に失敗 |
| 2 | `broadFileSystemAccess` を宣言 | フルトラストアプリには不要かつ制限付き capability |
| 3 | `uap10:RuntimeBehavior` / `TrustLevel` が未宣言 | 実行形態が暗黙的 |

## 1. WebView2 の依存宣言

**誤り:**

```xml
<uap10:HostRuntimeDependency
  Name="Microsoft.Win32WebView2"
  MinVersion="92.0.902.78"
  Publisher="CN=Microsoft Corporation, O=Microsoft Corporation, ..." />
```

**正しい:**

```xml
<win32dependencies:ExternalDependency
  Name="Microsoft.WebView2"
  Publisher="CN=Microsoft Windows, O=Microsoft Corporation, L=Redmond, S=Washington, C=US"
  MinVersion="92.0.902.78"
  Optional="true" />
```

要素名・Name・Publisher の**3 つすべてが誤り**でした。

- 要素は `uap10:HostRuntimeDependency` ではなく `win32dependencies:ExternalDependency`
- Name は `Microsoft.Win32WebView2` ではなく `Microsoft.WebView2`
- Publisher は `CN=Microsoft Windows, ...`（`Windows` が入る）

Microsoft の許可リストに載っているのは `Microsoft.WebView2` のみです
（[win32dependencies:ExternalDependency](https://learn.microsoft.com/en-us/uwp/schemas/appxpackage/uapmanifestschema/element-win32dependencies-externaldependency)）。

**この環境で再現しました。** 誤った宣言では次のエラーになります。

```
Add-AppxPackage -Register AppxManifest.xml
→ 0x80073D31: パッケージでホストのランタイムの依存関係または競合の検証に失敗しました。
```

修正後はこのエラーが消え、次の段階（開発者モード）に進みます。

## 2. broadFileSystemAccess の削除

**誤り:**

```xml
<rescap:Capability Name="runFullTrust" />
<rescap:Capability Name="broadFileSystemAccess" />
```

**正しい:**

```xml
<rescap:Capability Name="runFullTrust" />
```

`broadFileSystemAccess` は **AppContainer アプリ向け**の capability です。
サンドボックス内のアプリに、本来持っていないファイルアクセスを与えるためのものです。

ZipScope は `uap10:TrustLevel="mediumIL"`（フルトラスト）で動くため、
**サインイン中のユーザーと同じファイルアクセスを最初から持っています**。
宣言する必要がなく、宣言すると制限付き capability として別途 Store の承認が必要になります。

## 3. RuntimeBehavior / TrustLevel の明示

**誤り:**

```xml
<Application Id="ZipScope" Executable="ZipScope.exe"
             EntryPoint="Windows.FullTrustApplication">
```

**正しい:**

```xml
<Application Id="ZipScope" Executable="ZipScope.exe"
             EntryPoint="Windows.FullTrustApplication"
             uap10:RuntimeBehavior="packagedClassicApp"
             uap10:TrustLevel="mediumIL">
```

旧式の `EntryPoint="Windows.FullTrustApplication"` だけでもアップロードは通りますが、
実行形態が暗黙的になります。Wails アプリは Win32 プロセスをフルトラストで動かすため、
`packagedClassicApp` + `mediumIL` が正しい組み合わせです。

この 2 属性は Windows 10 version 2004 (build 19041) 以降が必須のため、
`MinVersion` を `10.0.17763.0` から `10.0.19041.0` に上げました。

## 比較: 認定を通った ZipSweep との差分

`archive-extract-cleaner`（ZipSweep）は WACK 24 テスト全項目 PASS で認定に進んでいます。
そのマニフェストと比較して差分を特定しました。

| 項目 | ZipSweep（認定通過） | ZipScope（差し戻し） |
| --- | --- | --- |
| RuntimeBehavior | `packagedClassicApp` | **未宣言** → 修正 |
| TrustLevel | `mediumIL` | **未宣言** → 修正 |
| Capabilities | `runFullTrust` のみ | `runFullTrust` + **`broadFileSystemAccess`** → 修正 |
| Dependencies | `TargetDeviceFamily` のみ | 不正な WebView2 宣言 → 修正 |
| MinVersion | 10.0.19041.0 | 10.0.17763.0 → 修正 |
| シェル拡張 | なし | COM 2 クラス + サムネイル + 右クリック |

**ZipSweep にはシェル拡張がありません。** ZipScope 固有の差分は COM 宣言ですが、
これはスキーマ上有効で、`runFullTrust` があれば登録できます。

## エクスプローラー統合の登録方法（調査結果）

懸念されていた「HKCU への COM 登録を前提とした `install.cmd`」は、
**パッケージには含まれていません**。

```
build/msix/verify/ に .cmd ファイルは無い
```

パッケージ版では、COM はマニフェストの `com:SurrogateServer` で宣言され、
インストール時に OS が登録します。`install.cmd` は GitHub Releases 版
（ポータブル実行）専用です。

| 配布経路 | COM 登録方法 |
| --- | --- |
| GitHub Releases | `install.cmd` → `regsvr32` → HKCU |
| Microsoft Store | マニフェストの `com:SurrogateServer` |

両者は独立しており、Store 版が `install.cmd` を実行することはありません。

## 修正版パッケージ

```
build/msix/ZipScope_0.1.1.0_x64.msix
```

| 項目 | 値 |
| --- | --- |
| Identity Name | `tonbiattack.ZipScope` |
| Publisher | `CN=2F14A66E-02DF-43E2-93E0-5C5900B38D2E` |
| Version | 0.1.1.0 |
| Package Family Name | `tonbiattack.ZipScope_e28ksxe3fc9w4` |

`scripts/verify-pfn.ps1` で Partner Center の値と一致することを確認済みです。

## 検証状況

| 項目 | 結果 |
| --- | --- |
| マニフェストのスキーマ検証 | PASS（`makeappx pack`） |
| 依存解決エラーの解消 | 確認済み（`0x80073D31` が消えた） |
| Identity / PFN の一致 | 確認済み |
| パッケージ内容の照合 | 全ファイル存在 |
| **インストール** | **未実施**（管理者権限が必要） |
| **WACK** | **未実施**（管理者権限が必要） |
| **アンインストール** | **未実施**（管理者権限が必要） |

### 未実施の理由

この環境には管理者権限がありません。以下がいずれも管理者権限を要求します。

- 自己署名証明書の `LocalMachine\TrustedPeople` への登録
- 開発者モードの有効化（`HKLM` への書き込み）
- Windows App Certification Kit（`appcert.exe` が管理者権限を要求）

```
Add-AppxPackage -Register AppxManifest.xml
→ 0x80073CFF: Windows 開発者用ライセンスまたはサイドローディング対応システムが必要です。
```

### 完了させる手順（管理者権限で実行）

```powershell
# 1. 署名済みパッケージを生成
pwsh scripts/make-msix.ps1 -Version 0.1.1.0 -Sign

# 2. 証明書を信頼
Import-Certificate -FilePath build\msix\ZipScope-Test.cer `
  -CertStoreLocation Cert:\LocalMachine\TrustedPeople

# 3. インストール
Add-AppxPackage -Path build\msix\ZipScope_0.1.1.0_x64.msix

# 4. 起動して基本操作を確認
Start-Process "shell:AppsFolder\tonbiattack.ZipScope_e28ksxe3fc9w4!ZipScope"

# 5. WACK を実行
& "C:\Program Files (x86)\Windows Kits\10\App Certification Kit\appcert.exe" `
  reset /testpak build\msix\ZipScope_0.1.1.0_x64.msix
& "C:\Program Files (x86)\Windows Kits\10\App Certification Kit\appcert.exe" `
  test /appx build\msix\ZipScope_0.1.1.0_x64.msix /reportoutput build\msix\wack.xml

# 6. アンインストール
Remove-AppxPackage -Package tonbiattack.ZipScope_0.1.1.0_x64__e28ksxe3fc9w4
```

開発者モードが無効な場合は、設定 → システム → 開発者向け → 開発者モードを ON にします。
