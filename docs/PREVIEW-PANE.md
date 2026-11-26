# エクスプローラーのプレビューペイン対応

ZIP / CBZ を選択するだけで、エクスプローラーのプレビューペイン（`Alt+P`）に
中の画像が一覧表示されます。ZipScope 本体は起動しません。

## 使い方

1. エクスプローラーで ZIP / CBZ を選択する
2. `Alt+P` でプレビューペインを開く（または表示 → プレビューウィンドウ）

| 操作 | 動作 |
| --- | --- |
| スクロール | 一覧をスクロール |
| クリック | サムネイルを選択 |
| `←` / `→` | 前 / 次の画像を選択 |
| `↑` / `↓` | 1 行上 / 下へ |
| `Home` / `End` | 最初 / 最後へ |
| `PageUp` / `PageDown` | ページ単位でスクロール |

枚数はヘッダに表示されます。ペインの幅に応じて列数が変わります
（110〜220 px のタイルに収まる範囲で自動調整）。

## 設計

`shell/preview_handler.cpp` に `IInitializeWithStream` / `IPreviewHandler` /
`IObjectWithSite` を実装しています。

### 全展開しない

ZIP の中央ディレクトリだけを読み、**表示に必要な画像だけ**をその場で展開します。
ディスクには一切書き出しません。150 枚のアーカイブでも、初期表示で展開するのは
画面に見えている十数枚だけです。

### 遅延読み込みとキャッシュ

サムネイルは可視領域のタイルについてのみデコードし、結果をメモリに保持します。
一度デコードしたタイルは再描画で再利用します。壊れたエントリは `failed` として
記憶し、毎回の描画で再試行しません。

### 古い画像を表示しない

アーカイブのバイト列・インデックス・サムネイルキャッシュは、
新しいファイルが選択された時点で**まとめて破棄**されます
（`Unload()` と `DoPreview()` の両方）。前のファイルの画像が残ることはありません。

### Explorer を固まらせない

処理はすべて UI スレッドで行いますが、1 回の描画でデコードするのは
可視タイルのみです。加えて次の上限を課しています。

| 制限 | 値 |
| --- | --- |
| アーカイブ全体のサイズ | 512 MiB |
| 1 エントリの展開サイズ | 128 MiB |
| 画素数 | 40 MP |
| 一覧に載せるエントリ数 | 2,000（超過分は「先頭のみ表示」と明示） |
| エントリの CRC | 不一致は表示しない |

破損 ZIP・画像なし ZIP・暗号化エントリは、ペイン内にメッセージを出すか
そのタイルだけプレースホルダにして、**Explorer は継続動作**します。

### キーボード操作の競合回避

`TranslateAccelerator` は、グリッドが実際に使うキー（矢印・Home/End・
PageUp/PageDown）だけを処理し、それ以外は `S_FALSE` を返して Explorer に渡します。
Tab・Esc・Explorer 自身のショートカットは奪いません。

## 登録

### 通常版（GitHub Releases / ポータブル）

`shell\install.cmd` を管理者として実行すると、以下が登録されます。

```
HKCU\Software\Classes\.zip\ShellEx\{8895b1c6-b41f-4c1c-a562-0d564250836f}
  (default)   = {6F1B2A12-8C3D-4E57-9A21-5B7C0D3E8F41}
  DisplayName = ZipScope

HKCU\Software\Classes\CLSID\{6F1B2A12-...}\InprocServer32 = <DLL パス>
HKCU\Software\Classes\CLSID\{6F1B2A12-...}\AppID          = {6F1B2A13-...}
HKCU\Software\Classes\AppID\{6F1B2A13-...}
  (default)   = ZipScope Preview Handler
  DllSurrogate = （空 = システムのサロゲートホストを使う）
```

**`DllSurrogate` が重要です。** Explorer はプレビューハンドラを
`explorer.exe` に直接読み込まず、サロゲートホスト経由で起動します。
これが無いとハンドラは一覧に出ても実行されません。

### Microsoft Store 版（MSIX）

パッケージではレジストリに書けないため、マニフェストで宣言します。

```xml
<uap:FileTypeAssociation Name="zipscope-zip">
  <uap:SupportedFileTypes>
    <uap:FileType>.zip</uap:FileType>
  </uap:SupportedFileTypes>
  <desktop2:ThumbnailHandler Clsid="6F1B2A10-..." />
  <desktop2:DesktopPreviewHandler Clsid="6F1B2A12-..." />
</uap:FileTypeAssociation>
```

COM クラスは `com:SurrogateServer` に `AppId` 付きで宣言します。

### 既存の関連付けへの影響

**ZipScope は既存のファイル関連付けを変更しません。** プレビューハンドラは
`.zip` / `.cbz` の `ShellEx` キーに追加されるだけで、既定のアプリや
Windows 標準の ZIP フォルダ機能には触れません。

`ShellEx` 配下の GUID `{8895b1c6-...}` はプレビューハンドラ用の固定値です。
他社のアーカイバが同じキーに登録している場合、**最後に登録したものが有効**に
なります（Windows の仕様）。ZipScope をアンインストールすると、このキーの
ZipScope の項目だけを削除します。

## 性能測定

`shell/previewbench.cpp` で初期表示時間とメモリを測定できます。

```powershell
# ZipScopeShell.dll をビルド・登録した後
previewbench.exe <archive.cbz> [more.cbz ...]
```

### 測定結果（2026-11-26、Windows 11 / SSD）

| アーカイブ | 枚数 | 初期表示 (ms) | ピーク作業セット (MB) |
| --- | --- | --- | --- |
| many-150.cbz | 150 | **23.4** | 10.3 |
| 01 山の風景.cbz | 8 | 23.1 | 10.6 |
| 03 夜の街.cbz | 10 | 22.9 | 11.5 |
| 05 砂丘.cbz | 12 | 23.9 | 11.0 |

ベースライン（プロセス起動直後）は 8.1 MB です。

- **枚数が増えても初期表示時間はほぼ変わらない**（23 ms 前後）。
  表示するのは可視タイルだけなので、150 枚でも 8 枚でも同じです。
- **メモリの増分は約 2〜3 MB**。全展開しないため、150 枚でもこの程度です。
- 5 回ずつ測定した中央値です。

## 実機検証

`shell/previewtest.cpp` が Explorer と同じ手順でハンドラを動かします。

```powershell
previewtest.exe <archive.cbz> <screenshot.png> [narrow.cbz] [reject.zip]
```

- COM 経由で生成 → ストリームを渡す → ウィンドウと矩形を渡す → 描画
- 広いペイン（520px）と狭いペイン（300px）で列数が変わることを確認
- ZIP でないファイルを渡してもクラッシュせずエラー状態を表示
- スクリーンショットを PNG で保存してレイアウトを目視確認

### 検証済み

| 項目 | 結果 |
| --- | --- |
| ハンドラの生成・初期化 | PASS |
| プレビュー描画 | PASS（`docs/store/preview-01.png`） |
| 狭いペインでのレイアウト | PASS（列数が減る） |
| 150 枚のアーカイブ | PASS（`docs/store/preview-many.png`） |
| 破損 ZIP | PASS（エラー表示、クラッシュなし） |
| 画像なし ZIP | PASS（エラー表示） |
| 登録内容 | PASS（HKCU の ShellEx / CLSID / AppID を確認） |
| DLL の読み込み | PASS（`tasklist /m ZipScopeShell.dll` で explorer.exe / dllhost.exe に現れる） |
| サムネイル・右クリック機能 | PASS（既存テストが継続して成功） |

### 解決済み: Explorer のペインに表示されなかった原因

最初の実装では Explorer からハンドラが呼ばれませんでした。原因は**登録先の不足**で、
2 点足りていませんでした。

#### 1. `PreviewHandlers` 一覧への登録（これが本質）

Windows は、`HKCU\Software\Microsoft\Windows\CurrentVersion\PreviewHandlers` に
載っているハンドラしか使いません。拡張子の `ShellEx` と CLSID だけでは**無視されます**。

```
HKCU\Software\Microsoft\Windows\CurrentVersion\PreviewHandlers
    {6F1B2A12-8C3D-4E57-9A21-5B7C0D3E8F41} = ZipScope Preview Handler
```

PowerToys（Monaco / Markdown / SVG）も同じ場所に登録されています。ここを見れば
「どのサードパーティ製ハンドラが有効か」が分かります。

#### 2. サロゲートホストの AppID

プレビューハンドラは `explorer.exe` ではなく **`prevhost.exe`** で動きます。
そのため CLSID に、Windows が HKLM で定義している
「Preview Handler Surrogate Host」の AppID を付ける必要があります。

```
HKCU\Software\Classes\CLSID\{6F1B2A12-...}
    AppID = {6D2B5079-2F0B-48DD-AB7F-97CEC514D30B}
```

HKLM 側の対応する AppID キーには `DllSurrogate = %SystemRoot%\system32\prevhost.exe`
が設定されています。自前の AppID を作って空の `DllSurrogate` を入れても動きません。

### 診断方法

Explorer の内側で何が起きているかは、DLL 側のログで確認できます。

```powershell
# トレースを有効にする（このファイルがある間だけ記録する）
New-Item -ItemType File -Path "$env:LOCALAPPDATA\ZipScope\preview.debug" -Force

# ファイルを選択してからログを見る
Get-Content "$env:LOCALAPPDATA\ZipScope\preview.log"
```

出力例（`prevhost.exe` で動いていることが分かる）:

```
16:12:03.221 pid=40364 tid=8120 prevhost.exe DLL loaded
16:12:03.222 pid=40364 tid=8120 prevhost.exe DllGetClassObject {6F1B2A12-...}
16:12:03.223 pid=40364 tid=8120 prevhost.exe ClassFactory::CreateInstance
16:12:03.225 pid=40364 tid=8120 prevhost.exe Initialize called
16:12:03.226 pid=40364 tid=8120 prevhost.exe SetWindow called
16:12:03.227 pid=40364 tid=8120 prevhost.exe DoPreview called
16:12:03.227 pid=40364 tid=8120 prevhost.exe DoPreview indexed 8 entries
```

`prevhost.exe` に `ZipScopeShell.dll` が読み込まれているかは次でも確認できます。

```powershell
tasklist /m ZipScopeShell.dll
```

**`prevhost.exe` は `LOCALAPPDATA` を環境変数として持っていません。** ログのパス解決は
`USERPROFILE` へのフォールバックが要ります（実装済み）。

### 調査で分かった、効かなかったこと

| 試したこと | 結果 |
| --- | --- |
| `HKCR\.zip\ShellEx\{8895b1c6}` のみ | 無視される |
| `SystemFileAssociations\.zip\ShellEx` を追加 | 効果なし |
| ProgID（`CompressedFolder`, `calibre64bit.AssocFile.CBZ`）の ShellEx を HKCU で上書き | 効果なし |
| 自前の AppID + 空の `DllSurrogate` | 効果なし |
| AppID を削除（インプロセス起動を期待） | 効果なし |
| `SHChangeNotify(SHCNE_ASSOCCHANGED)` | 単体では効果なし（登録が揃ってから意味を持つ） |
| `PreviewHandlers` 一覧 + システムのサロゲート AppID | **動作した** |

## 設定（オプトイン）

プレビュー統合は**拡張子ごとのオプトイン**です。インストール直後は無効で、
設定画面のチェックボックスで有効にします。

| 操作 | 場所 |
| --- | --- |
| 有効化・無効化 | ZipScope の設定 → 「エクスプローラーのプレビュー」 |
| コマンドから | `zipscope-preview enable zip` / `disable cbz` / `status` |

有効にすると、次回からは**ファイルを選択するだけ**で画像一覧が出ます
（ドロップダウンでの毎回の選択は不要）。

### 元の状態への復元

有効化したとき、その拡張子に**以前登録されていたハンドラの CLSID を記憶**します
（`HKCU\Software\ZipScope\preview-backup\.<ext>`）。無効化すると次を行います。

1. 記憶した CLSID を元に戻す
2. 元が空だった場合は、自分が作ったキーだけを削除する
3. どの拡張子も無効になったら `PreviewHandlers` の項目を削除する
4. バックアップを削除する

**他のアプリが登録しているハンドラは削除しません。** バックアップが無い状態で
無効化された場合も、登録値が ZipScope の CLSID のときだけ削除します。

アンインストール（`regsvr32 /u` → `DllUnregisterServer`）でも同じ復元を行います。

### Windows 標準の ZIP 機能への影響

**ありません。** 触るのは `ShellEx\{8895b1c6-...}`（プレビュー専用のスロット）だけです。

| 項目 | 場所 | 変化 |
| --- | --- | --- |
| ZIP の展開（すべて展開） | `CompressedFolder\ShellEx\ContextMenuHandlers` | 変更なし |
| ドラッグ＆ドロップ | `CompressedFolder\ShellEx\DropHandler` | 変更なし |
| 既定のアプリ | `.zip` の `UserChoice`（`CompressedFolder`） | 変更なし |
| フォルダとして開く | `CompressedFolder` の StorageHandler | 変更なし |
| サムネイル | `ShellEx\{E357FCCD-...}` | 変更なし |

実機で `.zip` の「すべて展開」がツールバーに出たままであること、
`UserChoice` が `CompressedFolder` のままであることを確認済みです。

## 検証結果（2026-11-26）

| 項目 | 結果 |
| --- | --- |
| CBZ 有効 → ペインに一覧 | **PASS**（Explorer 実機、`prevhost.exe` 上で動作） |
| ZIP 無効 → Windows 標準のプレビュー | **PASS**（一覧は出ない） |
| ZIP 有効 → ペインに一覧 | **PASS** |
| 無効化で元に戻る | **PASS**（バックアップ・一覧項目ともに残らない） |
| 二重有効化でバックアップが壊れない | **PASS**（`<absent>` を保持） |
| `disable-all` でクリーンアップ | **PASS** |
| Windows 標準の ZIP 展開 | **PASS**（未変更） |
| 既存のサムネイル・右クリック | **PASS**（回帰なし） |

## Microsoft Store（MSIX）版

パッケージでは HKCU に書けないため、マニフェストで宣言します。

```xml
<uap:FileTypeAssociation Name="zipscope-zip">
  <uap:SupportedFileTypes><uap:FileType>.zip</uap:FileType></uap:SupportedFileTypes>
  <desktop2:ThumbnailHandler Clsid="6F1B2A10-..." />
  <desktop2:DesktopPreviewHandler Clsid="6F1B2A12-..." />
</uap:FileTypeAssociation>
```

```xml
<com:SurrogateServer DisplayName="ZipScope Preview Handler"
                     AppId="6F1B2A13-...">
  <com:Class Id="6F1B2A12-..." Path="ZipScopeShell.dll" ThreadingModel="STA" />
</com:SurrogateServer>
```

**Store 版はオプトインにできません。** マニフェストで宣言した時点で、
インストール後に常に有効になります（パッケージの宣言はユーザーが個別に
切り替えられない）。この違いは既知の制約として受け入れます。

Store 版が有効な状態でも、Windows 標準の ZIP 展開には影響しません
（同じくプレビュー専用スロットだけを使うため）。