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

### 実機検証の結果（2026-11-24）

管理者権限で `scripts/verify-msix-install.ps1` を実行し、**全項目 PASS** しました。

| 項目 | 結果 |
| --- | --- |
| 証明書の信頼 | **PASS** |
| パッケージの署名 | **PASS**（信頼済み証明書で検証成功） |
| インストール | **PASS**（`tonbiattack.ZipScope_0.1.1.0_x64__e28ksxe3fc9w4`） |
| 起動 | **PASS**（pid 35420、36.4 MB） |
| WACK | **PASS**（24 テスト、必須チェック全 PASS） |
| アンインストール | **PASS** |

```
[PASS] Trust the test certificate   already trusted
[PASS] Package is signed            signature verifies against the trusted certificate
[PASS] Install the package          installed tonbiattack.ZipScope_0.1.1.0_x64__e28ksxe3fc9w4
[PASS] Launch the packaged app      pid 35420, 36.4 MB
[PASS] Windows App Certification Kit  24 tests, all required checks PASS; 1 optional finding(s)
[PASS] Uninstall the package        removed

Install, launch and uninstall all passed.
```

**認定項目 10.3.4 が問題にしていた「Store 経由でインストールできない」は解消しました。**

### WACK の結果詳細

24 テスト中 **23 PASS、必須チェックの失敗 0 件**です。

`OPTIONAL="TRUE"` の 1 件（ブロック済みの実行可能ファイル）が FAIL ですが、
**Microsoft はこれを認定要件にしていません**。指摘内容は次のとおりで、いずれも無害です。

| 指摘 | 評価 |
| --- | --- |
| `ZipScopeShell.dll` が `CreateProcessW` を参照 | **意図した動作**。右クリックメニューからビューアを起動するために必要 |
| `README.md` に `cmd` の文字列 | ドキュメント内の説明文 |
| `ZipScope.exe` に `REG` / `bash` / `CSI` / `powershell` / `Cmd` | Wails / WebView2 ランタイムに含まれる文字列。ZipScope 自身のコードではない |

このテストは「プロセス起動 API とブロックされた実行可能ファイルへの参照」を
文字列検索で検出するもので、誤検知が多く `OPTIONAL` に分類されています。

### WACK レポートの形式（判明したこと）

実際のレポートは次の構造でした。**`STATUS` 要素は存在しません。**

```xml
<REPORT>
  <REQUIREMENTS>
    <REQUIREMENT NUMBER="3" TITLE="...">
      <TEST INDEX="2" NAME="..." DESCRIPTION="..." OPTIONAL="TRUE">
        <MESSAGES>
          <MESSAGE TEXT="指摘内容" />
        </MESSAGES>
        <RESULT><![CDATA[PASS]]></RESULT>
      </TEST>
    </REQUIREMENT>
  </REQUIREMENTS>
</REPORT>
```

判定は `TEST/RESULT` の CDATA、指摘は `MESSAGE` の `TEXT` 属性です。
`OPTIONAL` 属性が `TRUE` のテストは認定要件外です。

最初のパーサーは `STATUS` を探していたため 0 件と判定し、
**FAIL があるのに PASS と表示する誤り**がありました。修正済みです。

### WACK の引数誤り

最初の実行では WACK が `exit -1` でレポートを出力しませんでした。
引数名が誤っていました。

```
# 誤り（exit -1、レポートなし）
appcert test /appx <path> /reportoutput <path>

# 正しい
appcert reset
appcert test -appxpackagepath <path> -reportoutputpath <path>
```

`reset` を先に実行して前回の状態を消す必要もあります。
ZipSweep の `msix-acceptance.yml` と同じ呼び方に合わせました。

### 終了コードの扱い

WACK は診断情報として扱い、**インストール・起動・アンインストール**の
3 項目で終了コードを決めるようにしました。WACK の失敗は
「パッケージがインストールできない」ことを意味しないためです。

### 完了させる手順（管理者権限で実行）

**1 コマンドで完結する検証スクリプトを用意しました。** どのディレクトリから実行しても動きます。

```powershell
# 管理者権限の PowerShell で
pwsh -File C:\apps\zip-scope\scripts\verify-msix-install.ps1
```

このスクリプトが次を順に実行し、各項目の PASS/FAIL を表示します。

1. テスト証明書を `LocalMachine\TrustedPeople` に信頼
2. `Add-AppxPackage` でインストール
3. 起動してプロセスが立ち上がることを確認
4. WACK を実行して全項目の合否を集計
5. アンインストールして削除を確認

オプション:

| オプション | 動作 |
| --- | --- |
| `-SkipWack` | WACK を省略（数分かかるため） |
| `-KeepInstalled` | インストールしたまま終了 |

手動で行う場合:

```powershell
# すべて絶対パスで指定する（相対パスは呼び出し元の cwd 基準になるため）
$repo = 'C:\apps\zip-scope'

Import-Certificate -FilePath "$repo\build\msix\ZipScope-Test.cer" `
  -CertStoreLocation Cert:\LocalMachine\TrustedPeople

Add-AppxPackage -Path "$repo\build\msix\ZipScope_0.1.1.0_x64.msix"

Start-Process "shell:AppsFolder\tonbiattack.ZipScope_e28ksxe3fc9w4!ZipScope"

& "C:\Program Files (x86)\Windows Kits\10\App Certification Kit\appcert.exe" `
  test /appx "$repo\build\msix\ZipScope_0.1.1.0_x64.msix" `
  /reportoutput "$repo\build\msix\wack-report.xml"

Remove-AppxPackage -Package tonbiattack.ZipScope_0.1.1.0_x64__e28ksxe3fc9w4
```

開発者モードが無効な場合は、設定 → システム → 開発者向け → 開発者モードを ON にします。

### 相対パスで失敗した例（実際に起きたこと）

`C:\Users\teni2` から相対パスで実行すると、すべてリポジトリ外を指します。

```
PS C:\Users\teni2> pwsh scripts/make-msix.ps1 -Version 0.1.1.0 -Sign
引数 'scripts/make-msix.ps1' はスクリプト ファイルの名前として認識されません。

PS C:\Users\teni2> Import-Certificate -FilePath build\msix\ZipScope-Test.cer ...
Import-Certificate: 証明書ファイルが見つかりませんでした。

PS C:\Users\teni2> Add-AppxPackage -Path build\msix\ZipScope_0.1.1.0_x64.msix
Add-AppxPackage: パス 'C:\Users\teni2\build\msix\ZipScope_0.1.1.0_x64.msix' は存在しないため、見つかりません。
```

スクリプト側を修正しました。`make-msix.ps1` と `verify-pfn.ps1` は
`$PSScriptRoot` からリポジトリを解決し、`verify-pfn.ps1` は引数を省略すると
最新のパッケージを自動で選びます。出力メッセージも絶対パスで案内します。

## 署名で失敗した原因（0x8007000B）

証明書を信頼した後、`Add-AppxPackage` が次のエラーになりました。

```
Add-AppxPackage: 0x800B0100: サブジェクトに署名がありませんでした。
```

パッケージが**署名されていなかった**ためです。`-Sign` なしで再ビルドしたためでした。

署名を試みると、今度は `signtool` が失敗しました。

```
SignTool Error: An unexpected internal error has occurred.
Error information: "Error: SignerSign() failed." (-2147024885/0x8007000b)
```

**`0x8007000B` は `ERROR_BAD_FORMAT` ですが、実際の原因は別です。**
イベントログに本当の理由が出ます（[SignTool の既知の問題](https://learn.microsoft.com/en-us/windows/msix/package/signing-known-issues)）。

| イベント ID | 意味 |
| --- | --- |
| 150 | マニフェストの Publisher と証明書の Subject が一致しない |
| 151 | `/fd` のハッシュ方式がブロックマップと一致しない |
| 152 | パッケージが壊れている |

今回は **150** でした。

```
マニフェスト: Publisher="CN=2F14A66E-02DF-43E2-93E0-5C5900B38D2E"  ← Partner Center の GUID
証明書:       Subject="CN=tonbiattack"                              ← 一致しない
```

**MSIX の署名証明書は、Subject がマニフェストの Publisher と完全一致している必要があります。**
これは Store 提出用のパッケージにも当てはまります（Store が再署名する場合でも、
アップロード時の検証で同じ照合が行われます）。

### 修正

`make-msix.ps1 -Sign` が、マニフェストの Publisher と同じ Subject の証明書を
自動で作成するようにしました。

```powershell
# 証明書の Subject は $Publisher（= CN=2F14A66E-...）になる
pwsh -File C:\apps\zip-scope\scripts\make-msix.ps1 -Version 0.1.1.0 -Sign
```

署名に失敗した場合は、イベントログの確認方法を含むエラーメッセージを出します。

### 検証結果

```
Signing Certificate Chain:
    Issued to: 2F14A66E-02DF-43E2-93E0-5C5900B38D2E
    Expires:   Fri Nov 24 13:20:17 2028
```

署名は正しく入りました。`signtool verify` が残すエラーは
「証明書チェーンが信頼されていない」のみで、これは
`LocalMachine\TrustedPeople` へのインポート（管理者権限）で解消します。
