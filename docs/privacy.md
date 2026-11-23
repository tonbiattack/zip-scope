# プライバシーポリシー / Privacy Policy

**ZipScope**（以下「本アプリ」）のプライバシーポリシーです。
最終更新日: 2026-11-23

---

## 日本語

### 1. ネットワーク通信を行いません

本アプリは**一切のネットワーク通信を行いません**。
インターネット接続なしで完全に動作します。

具体的には、以下を行いません。

- 閲覧したファイル名・パス・画像の外部送信
- 利用状況・操作履歴の収集
- クラッシュレポートの自動送信
- アクセス解析・テレメトリ
- 広告の配信
- アカウントの作成やログイン

### 2. 端末内に保存される情報

本アプリは、動作に必要なデータを**お使いの端末内にのみ**保存します。
保存先は `%LOCALAPPDATA%\ZipScope` です。

| 保存されるもの | 内容 | 目的 |
| --- | --- | --- |
| `index.db` | ZIP のパス・サイズ・更新日時・エントリ一覧 | 再起動時の一覧表示を高速化 |
| `thumbs/` | ZIP 内の画像から生成した縮小画像 | サムネイル表示を高速化 |
| `settings.json` | 最後に開いたフォルダ・表示設定 | 前回の状態の復元 |

これらは外部に送信されません。アプリをアンインストールするか、
上記フォルダを削除すると完全に消去されます。

### 3. 読み取るファイル

本アプリは、**利用者が明示的に選択したフォルダ**と、そこで見つかった
ZIP / CBZ ファイルのみを読み取ります。

- 元の ZIP ファイルは**読み取り専用**で開き、変更しません
- ファイルの移動・名前変更・削除は、利用者が明示的に操作した場合のみ実行します
- 削除は**ごみ箱への移動**で、確認ダイアログを経由します

### 4. エクスプローラー統合

本アプリはオプションでエクスプローラー統合（サムネイル表示・右クリックメニュー）を
提供します。この機能は以下を行います。

- ZIP / CBZ のアイコン用に、中の最初の画像からサムネイルを生成
- 右クリックメニューに「ZipScopeで開く」を追加

この処理も端末内で完結し、外部通信は行いません。

### 5. 第三者への提供

本アプリは、いかなる個人情報も第三者に提供・販売・共有しません。
そもそも収集していません。

### 6. お子様のプライバシー

本アプリは個人情報を収集しないため、年齢を問わず安全にご利用いただけます。

### 7. Microsoft Store 経由の配布について

Microsoft Store からインストールした場合、Store 自体が取得する情報
（インストール数・クラッシュ統計など）については
[Microsoft のプライバシーステートメント](https://privacy.microsoft.com/ja-jp/privacystatement)
が適用されます。本アプリの開発者は、これらの情報に個人を特定できる形でアクセスしません。

### 8. ポリシーの変更

本ポリシーを変更する場合は、このページを更新し、最終更新日を改定します。

### 9. お問い合わせ

ご質問・ご指摘は GitHub の Issue へお願いします。

- https://github.com/tonbiattack/zip-scope/issues

---

## English

### 1. No network communication

ZipScope does **not make any network connections**. It works fully offline.

Specifically, the app does not:

- transmit file names, paths, or images
- collect usage data or telemetry
- send crash reports
- serve advertising
- require an account or sign-in

### 2. Data stored on your device

The app stores only what it needs to work, and only on your device,
under `%LOCALAPPDATA%\ZipScope`:

| Stored | Contents | Purpose |
| --- | --- | --- |
| `index.db` | archive paths, sizes, timestamps, entry lists | fast startup |
| `thumbs/` | downscaled images generated from archives | fast thumbnails |
| `settings.json` | last folder and display preferences | restore the previous session |

None of this leaves your device. Removing the app or deleting that folder
erases it completely.

### 3. Files the app reads

The app reads only the folder you explicitly choose and the ZIP/CBZ files
found there.

- Archives are opened **read-only** and never modified.
- Moving, renaming, or deleting a file happens only when you ask for it.
- Deletion goes to the **Recycle Bin** and requires confirmation.

### 4. Explorer integration

The optional Explorer integration generates thumbnails for ZIP/CBZ icons and
adds a "ZipScopeで開く" context menu entry. It also runs entirely on your
device.

### 5. Third parties

The app does not collect personal information, so there is nothing to share,
sell, or disclose.

### 6. Children's privacy

Because no personal information is collected, the app is safe for all ages.

### 7. Distribution through the Microsoft Store

If you install from the Microsoft Store, information the Store itself collects
(install counts, crash statistics) is governed by the
[Microsoft Privacy Statement](https://privacy.microsoft.com/en-us/privacystatement).
The developer does not access that information in an identifiable form.

### 8. Changes

Any change to this policy is published on this page with a new "last updated"
date.

### 9. Contact

Please open an issue at https://github.com/tonbiattack/zip-scope/issues
