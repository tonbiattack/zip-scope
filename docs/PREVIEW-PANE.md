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
| サムネイル・右クリック機能 | PASS（既存テストが継続して成功） |

### 未検証

Explorer 本体のプレビューペインでの表示は、この環境では確認できていません
（`Alt+P` の操作と Explorer の再起動が必要）。ハンドラ単体の動作、
登録内容、既存機能との共存は上記のとおり確認済みです。

## 実装中に見つけた不具合

**WIC の遅延読み込みで解放済みメモリを参照していました。**
サムネイルのバイト列をローカル変数に置いていたため、ループを抜けた時点で
解放され、後から `CopyPixels` が解放済み領域を読んでいました
（症状: 「Not a JPEG file」が並び、画像が表示されない）。

対策として、展開したバイト列をキャッシュエントリ内に保持するようにしました。

```cpp
struct CachedThumb {
    std::vector<BYTE> bytes;   // WIC が参照している間、生かしておく
    ComPtr<IWICBitmapSource> source;
    ...
};
```
