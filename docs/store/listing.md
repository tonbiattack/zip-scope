# Microsoft Store 掲載素材

Partner Center の申請フォームに入力する内容です。
スクリーンショットは `docs/store/screenshots/` にあります。

## スクリーンショット

すべて 1920×1080（Store 推奨サイズ）。自作の図形のみで構成したサンプルを使用しており、
第三者の著作物は含みません。

| ファイル | 内容 | 用途 |
| --- | --- | --- |
| `01-folder.png` | フォルダ内の CBZ 一覧（大アイコン表示） | 1 枚目。表紙が並ぶ様子 |
| `02-archive.png` | ZIP 内の画像グリッド（パンくずに ZIP バッジ） | 2 枚目。アーカイブの中身 |
| `03-viewer.png` | 原寸ビュー（フィット表示） | 3 枚目。閲覧画面 |
| `04-explorer.png` | エクスプローラーでのサムネイル表示 | 4 枚目。OS 統合 |

サンプルデータの生成:

```powershell
go run ./cmd/mkscreenshots "$env:USERPROFILE\Pictures\ZipScope サンプル"
```

図形のみを描画するため、権利上の問題はありません。

## アプリ名

```
ZipScope
```

## 短い説明（100 文字以内）

```
ZIP や CBZ を解凍せずに、中の画像をそのまま閲覧できるビューアです。大量のアーカイブでも素早く一覧でき、エクスプローラーとも連携します。
```

## 説明（日本語）

```
ZipScope は、ZIP や CBZ を解凍せずに中の画像を閲覧できる Windows 用のビューアです。
大量のアーカイブを扱うときも、サムネイルが素早く並び、クリックだけで読めます。

■ ZIP を展開しない
アーカイブの中央ディレクトリだけを読み、表示に必要な画像だけを取り出します。
ディスクを圧迫せず、大きなアーカイブでも待たされません。

■ エクスプローラーと連携
ZIP / CBZ のアイコンが中の最初の画像になります。右クリックメニューの
「ZipScopeで開く」から直接開けます。

■ 1 万件でも軽快
画面内だけを描画する仮想スクロールを採用しています。数千のアーカイブを
含むフォルダでも、スクロールが引っかかりません。

■ 2 回目からはもっと速い
サムネイルをディスクにキャッシュします。一度見たアーカイブは、
次回から一瞬で表示されます。キャッシュは設定から削除できます。

■ オフラインで完結
ネットワーク通信は一切行いません。閲覧した内容が外部に送られることはありません。

■ 主な機能
・フォルダを選ぶだけで ZIP / CBZ を自動検出
・名前・更新日時・サイズでの並び替え、検索
・詳細 / 中アイコン / 大アイコンの 3 表示
・原寸ビュー（フィット・等倍・ズーム・パン・キーボード操作）
・ZIP の名前変更・移動・ごみ箱への削除
・ダークテーマ

■ 対応形式
ZIP / CBZ（Store / Deflate、ZIP64）
JPEG / PNG / GIF（静止画）/ WebP

■ 動作環境
Windows 10 version 1809 以降 / Windows 11
WebView2 ランタイム（Windows 11 には標準搭載）
```

## Description (English)

```
ZipScope lets you browse the images inside ZIP and CBZ archives without
extracting them. Thumbnails appear immediately, and a single click opens a page.

■ No extraction
Only the archive's central directory is read; just the images you view are
pulled out. Your disk stays clean and large archives open instantly.

■ Works with File Explorer
ZIP and CBZ files show their first image as the icon, and a "ZipScopeで開く"
entry in the right-click menu opens them directly.

■ Fast with thousands of files
Virtual scrolling renders only what is on screen, so a folder with thousands of
archives still scrolls smoothly.

■ Faster the second time
Thumbnails are cached on disk. Archives you have opened before appear instantly,
and the cache can be cleared from settings.

■ Fully offline
The app makes no network connections. Nothing you view leaves your device.

■ Features
- Detects ZIP/CBZ files in any folder you choose
- Sort by name, date or size; search
- Details, medium icon and large icon views
- Full-size viewer with fit, 1:1, zoom, pan and keyboard navigation
- Rename, move and recycle archives
- Dark theme

■ Supported formats
ZIP / CBZ (Store / Deflate, ZIP64)
JPEG / PNG / GIF (still images) / WebP

■ Requirements
Windows 10 version 1809 or later / Windows 11
WebView2 Runtime (included in Windows 11)
```

## カテゴリ

```
写真 > ビューア
```

代替: `ユーティリティ > ファイル管理`

## システム要件

| 項目 | 値 |
| --- | --- |
| OS | Windows 10 version 1809 (build 17763) 以降 / Windows 11 |
| アーキテクチャ | x64 |
| メモリ | 4 GB 以上を推奨 |
| ディスク | 50 MB（キャッシュは別途、初期上限 2 GB） |
| ランタイム | WebView2（パッケージが依存として宣言） |

## 年齢評価

全年齢。暴力・性的表現・薬物・賭博・恐怖表現を含みません。
利用者が自分で選んだ画像を表示するのみです。

## プライバシーポリシー URL

```
https://tonbiattack.github.io/zip-scope/privacy.html
```

## サポート URL

```
https://github.com/tonbiattack/zip-scope/issues
```

## ウェブサイト

```
https://github.com/tonbiattack/zip-scope
```

## 著作権情報

```
MIT License / Copyright (c) 2026 tonbiattack
```

## 検索キーワード

```
ZIP, CBZ, 画像, ビューア, アーカイブ, 漫画, コミック, サムネイル, 解凍不要
```

## 提出前チェックリスト

- [x] スクリーンショット 4 枚（1920×1080）
- [x] 説明文（日本語・英語）
- [x] プライバシーポリシー URL（公開済み）
- [x] サポート URL
- [x] アイコン一式（`msix/Assets/`）
- [x] パッケージ（`scripts/make-msix.ps1`）
- [ ] 実機でのサイドロード検証（#17）
- [ ] Partner Center への入力（本人の操作）
