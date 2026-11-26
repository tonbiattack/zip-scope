// Preview handler implementation: a scrollable thumbnail grid inside Explorer's
// preview pane.
//
// Design notes that matter for Explorer's stability:
//
//   - All work happens on the UI thread. Explorer tolerates a short delay while
//     the pane fills but not a blocked message pump, so the work per paint is
//     bounded: only visible tiles are decoded, and each decode is capped.
//   - The archive bytes and index are replaced wholesale when a new file is
//     selected. Cached thumbnails live in a vector that is cleared at the same
//     time, so an image from the previous file can never be drawn.
//   - A broken or encrypted entry shows a placeholder rather than failing the
//     preview, and a broken archive shows a message rather than an empty pane.
//   - Nothing is written to disk and ZipScope.exe is never started.
#include "preview_handler.h"

#include <windowsx.h>
#include <wincodec.h>
#include <wrl/client.h>

#include <algorithm>
#include <string>
#include <vector>

#include "decode.h"
#include "dllmain.h"
#include "guids.h"

#pragma comment(lib, "windowscodecs.lib")

using Microsoft::WRL::ComPtr;

namespace zipscope {

namespace {

// Layout values in device-independent pixels, scaled by the pane's DPI.
constexpr int kTileMinWidth = 110;
constexpr int kTileMaxWidth = 220;
constexpr int kGap = 8;
constexpr int kLabelHeight = 16;
constexpr int kHeaderHeight = 24;

// kPreviewMaxEntries caps the index. A ZIP with a million image entries must not
// build a million-row list inside Explorer. (The whole-archive byte cap is the
// shared kMaxArchiveBytes from guids.h.)
constexpr size_t kPreviewMaxEntries = 2000;

// Scale converts a DIP to physical pixels.
int Scale(int dip, UINT dpi) { return MulDiv(dip, static_cast<int>(dpi), 96); }

// CachedThumb is one decoded thumbnail. failed marks an entry that could not be
// decoded, so the pane does not retry it on every paint.
//
// bytes holds the compressed entry data and must outlive source: WIC's decoder
// reads from the memory buffer lazily, so freeing the buffer while the scaler is
// alive would make the later CopyPixels read freed memory.
struct CachedThumb {
    std::vector<BYTE> bytes;
    ComPtr<IWICBitmapSource> source;
    int width = 0;
    int height = 0;
    bool failed = false;
};

}  // namespace

// ---------------------------------------------------------------------------
// Impl: the child window and its state
// ---------------------------------------------------------------------------

struct PreviewHandler::Impl {
    HWND hwnd = nullptr;
    UINT dpi = 96;
    HFONT font = nullptr;
    HFONT fontSmall = nullptr;

    std::vector<BYTE> bytes;
    ZipArchive zipped;
    bool loaded = false;
    bool truncated = false;
    std::wstring error;

    // Cleared whenever the archive changes. Cleared to the same length as the
    // entry count so an index is always in range.
    std::vector<CachedThumb> thumbs;

    int clientWidth = 0;
    int clientHeight = 0;
    int columns = 1;
    int tileWidth = kTileMinWidth;
    int tileHeight = kTileMinWidth + kLabelHeight;
    int scrollY = 0;
    int selected = 0;
    bool focused = false;

    void Relayout();
    int ScrollMax() const;
    void ScrollTo(int offset);
    void EnsureVisible(int index);
    void DecodeVisible(HDC dc);
    void Paint(HDC dc, const RECT& client);
    void MoveSelection(int delta);

    static LRESULT CALLBACK Proc(HWND hwnd, UINT msg, WPARAM wParam, LPARAM lParam);
};

void PreviewHandler::Impl::Relayout() {
    int gap = Scale(kGap, dpi);
    int minTile = Scale(kTileMinWidth, dpi);
    int maxTile = Scale(kTileMaxWidth, dpi);

    int width = clientWidth > 0 ? clientWidth : minTile + gap * 2;
    columns = (std::max)(1, (width + gap) / (minTile + gap));
    tileWidth = (std::max)(minTile, (width - gap * (columns + 1)) / columns);
    tileWidth = (std::min)(tileWidth, maxTile);
    tileHeight = tileWidth + Scale(kLabelHeight, dpi);
}

int PreviewHandler::Impl::ScrollMax() const {
    if (!loaded) return 0;
    int rows = (static_cast<int>(zipped.Count()) + columns - 1) / columns;
    int gap = Scale(kGap, dpi);
    int header = Scale(kHeaderHeight, dpi);
    int content = header + rows * (tileHeight + gap) + gap;
    return (std::max)(0, content - clientHeight);
}

void PreviewHandler::Impl::ScrollTo(int offset) {
    int clamped = (std::max)(0, (std::min)(offset, ScrollMax()));
    if (clamped == scrollY) return;
    scrollY = clamped;
    if (hwnd) InvalidateRect(hwnd, nullptr, FALSE);
}

void PreviewHandler::Impl::EnsureVisible(int index) {
    if (columns <= 0) return;
    int gap = Scale(kGap, dpi);
    int header = Scale(kHeaderHeight, dpi);
    int row = index / columns;
    int top = header + row * (tileHeight + gap) - scrollY;
    int bottom = top + tileHeight;
    if (top < 0) {
        ScrollTo(scrollY + top);
    }
    else if (bottom > clientHeight) {
        ScrollTo(scrollY + bottom - clientHeight);
    }
}

void PreviewHandler::Impl::MoveSelection(int delta) {
    if (!loaded || zipped.Count() == 0) return;
    int count = static_cast<int>(zipped.Count());
    int next = (std::max)(0, (std::min)(selected + delta, count - 1));
    if (next == selected) return;
    selected = next;
    EnsureVisible(selected);
    if (hwnd) InvalidateRect(hwnd, nullptr, FALSE);
}

// DecodeVisible fills in thumbnails for the tiles that are about to be drawn.
// It is called from the paint handler, so it must stay bounded: only the rows
// intersecting the viewport, plus nothing beyond them.
void PreviewHandler::Impl::DecodeVisible(HDC dc) {
    if (!loaded || thumbs.size() != zipped.Count()) return;

    int gap = Scale(kGap, dpi);
    int header = Scale(kHeaderHeight, dpi);
    int step = tileHeight + gap;

    // The first and last rows touching the client area.
    int firstRow = (std::max)(0, (scrollY - header) / step);
    int lastRow = (scrollY + clientHeight - header) / step + 1;
    int count = static_cast<int>(zipped.Count());

    for (int row = firstRow; row <= lastRow; row++) {
        for (int col = 0; col < columns; col++) {
            int index = row * columns + col;
            if (index < 0 || index >= count) continue;
            CachedThumb& thumb = thumbs[index];
            if (thumb.source || thumb.failed) continue;

            // The bytes are stored in the cache entry, not a local: WIC keeps a
            // pointer into this buffer for as long as the scaler is alive.
            if (!zipped.Extract(index, &thumb.bytes)) {
                thumb.failed = true;
                continue;
            }
            std::vector<BYTE>& image = thumb.bytes;

            // Decode with WIC and scale to the tile. Failure is remembered so a
            // corrupt entry is not retried on every paint.
            ComPtr<IWICImagingFactory> factory;
            if (FAILED(CoCreateInstance(CLSID_WICImagingFactory, nullptr, CLSCTX_INPROC_SERVER,
                                        IID_PPV_ARGS(&factory)))) {
                thumb.failed = true;
                continue;
            }
            ComPtr<IWICStream> stream;
            ComPtr<IWICBitmapDecoder> decoder;
            ComPtr<IWICBitmapFrameDecode> frame;
            ComPtr<IWICFormatConverter> converter;
            ComPtr<IWICBitmapScaler> scaler;
            if (FAILED(factory->CreateStream(&stream)) ||
                FAILED(stream->InitializeFromMemory(image.data(), static_cast<DWORD>(image.size()))) ||
                FAILED(factory->CreateDecoderFromStream(stream.Get(), nullptr,
                                                        WICDecodeMetadataCacheOnDemand, &decoder)) ||
                FAILED(decoder->GetFrame(0, &frame))) {
                thumb.failed = true;
                continue;
            }

            UINT srcW = 0, srcH = 0;
            if (FAILED(frame->GetSize(&srcW, &srcH)) || srcW == 0 || srcH == 0) {
                thumb.failed = true;
                continue;
            }

            UINT dstW = static_cast<UINT>(tileWidth);
            UINT dstH = static_cast<UINT>(tileWidth);
            if (srcW >= srcH) {
                dstH = std::max<UINT>(1, static_cast<UINT>(
                    static_cast<uint64_t>(srcH) * tileWidth / srcW));
            }
            else {
                dstW = std::max<UINT>(1, static_cast<UINT>(
                    static_cast<uint64_t>(srcW) * tileWidth / srcH));
            }

            if (FAILED(factory->CreateFormatConverter(&converter)) ||
                FAILED(converter->Initialize(frame.Get(), GUID_WICPixelFormat32bppPBGRA,
                                             WICBitmapDitherTypeNone, nullptr, 0.0,
                                             WICBitmapPaletteTypeCustom)) ||
                FAILED(factory->CreateBitmapScaler(&scaler)) ||
                FAILED(scaler->Initialize(converter.Get(), dstW, dstH,
                                          WICBitmapInterpolationModeFant))) {
                thumb.failed = true;
                continue;
            }

            thumb.source = scaler;
            thumb.width = static_cast<int>(dstW);
            thumb.height = static_cast<int>(dstH);
        }
    }
    (void)dc;
}

void PreviewHandler::Impl::Paint(HDC dc, const RECT& client) {
    // Double buffer so scrolling and selection changes do not flicker.
    HDC mem = CreateCompatibleDC(dc);
    HBITMAP bmp = CreateCompatibleBitmap(dc, client.right, client.bottom);
    HGDIOBJ oldBmp = SelectObject(mem, bmp);

    // Dark background, matching the app.
    HBRUSH bg = CreateSolidBrush(RGB(24, 24, 28));
    FillRect(mem, &client, bg);
    DeleteObject(bg);

    SetBkMode(mem, TRANSPARENT);

    if (!error.empty()) {
        SetTextColor(mem, RGB(180, 180, 190));
        HGDIOBJ oldFont = SelectObject(mem, font);
        RECT rc = client;
        DrawTextW(mem, error.c_str(), -1, &rc, DT_CENTER | DT_VCENTER | DT_WORDBREAK);
        SelectObject(mem, oldFont);
    }
    else if (!loaded) {
        SetTextColor(mem, RGB(140, 140, 150));
        HGDIOBJ oldFont = SelectObject(mem, font);
        RECT rc = client;
        DrawTextW(mem, L"プレビューを読み込んでいます…", -1, &rc,
                  DT_CENTER | DT_VCENTER | DT_SINGLELINE);
        SelectObject(mem, oldFont);
    }
    else {
        // Header: image count and a truncation note.
        int header = Scale(kHeaderHeight, dpi);
        wchar_t headerText[128] = {};
        if (truncated) {
            swprintf_s(headerText, L"%zu 枚（先頭のみ表示）", zipped.Count());
        }
        else {
            swprintf_s(headerText, L"%zu 枚", zipped.Count());
        }
        SetTextColor(mem, RGB(150, 150, 165));
        HGDIOBJ oldFont = SelectObject(mem, fontSmall);
        RECT headerRect = {Scale(kGap, dpi), 4, client.right - Scale(kGap, dpi), header};
        DrawTextW(mem, headerText, -1, &headerRect, DT_LEFT | DT_VCENTER | DT_SINGLELINE);
        SelectObject(mem, oldFont);

        // Decode whatever is about to be drawn.
        DecodeVisible(mem);

        int gap = Scale(kGap, dpi);
        int step = tileHeight + gap;
        int count = static_cast<int>(zipped.Count());

        for (int row = 0; row * columns < count; row++) {
            int top = header + row * step - scrollY;
            if (top + tileHeight < header || top > client.bottom) continue;

            for (int col = 0; col < columns; col++) {
                int index = row * columns + col;
                if (index >= count) break;

                int left = gap + col * (tileWidth + gap);
                RECT tile = {left, top, left + tileWidth, top + tileWidth};

                // Tile background.
                HBRUSH tileBg = CreateSolidBrush(RGB(36, 36, 42));
                FillRect(mem, &tile, tileBg);
                DeleteObject(tileBg);

                const CachedThumb& thumb = thumbs[index];
                if (thumb.source) {
                    // Draw the scaled bitmap centred in the tile.
                    uint64_t stride = static_cast<uint64_t>(thumb.width) * 4;
                    std::vector<BYTE> pixels(static_cast<size_t>(stride) * thumb.height);
                    WICRect rc = {0, 0, thumb.width, thumb.height};
                    if (SUCCEEDED(thumb.source->CopyPixels(&rc, static_cast<UINT>(stride),
                                                           static_cast<UINT>(pixels.size()),
                                                           pixels.data()))) {
                        BITMAPINFO bi = {};
                        bi.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
                        bi.bmiHeader.biWidth = thumb.width;
                        bi.bmiHeader.biHeight = -thumb.height;  // top-down
                        bi.bmiHeader.biPlanes = 1;
                        bi.bmiHeader.biBitCount = 32;
                        bi.bmiHeader.biCompression = BI_RGB;

                        int x = left + (tileWidth - thumb.width) / 2;
                        int y = top + (tileWidth - thumb.height) / 2;
                        SetDIBitsToDevice(mem, x, y, thumb.width, thumb.height, 0, 0, 0,
                                          thumb.height, pixels.data(), &bi, DIB_RGB_COLORS);
                    }
                }
                else if (thumb.failed) {
                    SetTextColor(mem, RGB(140, 140, 150));
                    HGDIOBJ f = SelectObject(mem, fontSmall);
                    RECT rc = tile;
                    DrawTextW(mem, L"表示できません", -1, &rc,
                              DT_CENTER | DT_VCENTER | DT_WORDBREAK);
                    SelectObject(mem, f);
                }

                // Selection border.
                if (index == selected) {
                    HPEN pen = CreatePen(PS_SOLID, 2, RGB(90, 140, 255));
                    HGDIOBJ oldPen = SelectObject(mem, pen);
                    HGDIOBJ oldBrush = SelectObject(mem, GetStockObject(NULL_BRUSH));
                    Rectangle(mem, tile.left, tile.top, tile.right, tile.bottom);
                    SelectObject(mem, oldBrush);
                    SelectObject(mem, oldPen);
                    DeleteObject(pen);
                }

                // File name under the tile.
                SetTextColor(mem, RGB(170, 170, 180));
                HGDIOBJ f = SelectObject(mem, fontSmall);
                std::string name = zipped.Entries()[index].name;
                size_t slash = name.find_last_of('/');
                if (slash != std::string::npos) name = name.substr(slash + 1);
                wchar_t wideName[260] = {};
                MultiByteToWideChar(CP_UTF8, 0, name.c_str(), -1, wideName,
                                    static_cast<int>(std::size(wideName)));
                RECT label = {left, top + tileWidth + 2, left + tileWidth,
                              top + tileWidth + Scale(kLabelHeight, dpi)};
                DrawTextW(mem, wideName, -1, &label,
                          DT_CENTER | DT_SINGLELINE | DT_END_ELLIPSIS);
                SelectObject(mem, f);
            }
        }

        // Scrollbar hint on the right edge when the content overflows.
        int maxScroll = ScrollMax();
        if (maxScroll > 0) {
            int trackTop = header;
            int trackHeight = client.bottom - header;
            int thumbHeight = (std::max)(24, trackHeight * clientHeight / (clientHeight + maxScroll));
            int thumbTop = trackTop + (trackHeight - thumbHeight) * scrollY / maxScroll;
            RECT bar = {client.right - 6, thumbTop, client.right - 3, thumbTop + thumbHeight};
            HBRUSH barBrush = CreateSolidBrush(RGB(80, 80, 92));
            FillRect(mem, &bar, barBrush);
            DeleteObject(barBrush);
        }
    }

    BitBlt(dc, 0, 0, client.right, client.bottom, mem, 0, 0, SRCCOPY);
    SelectObject(mem, oldBmp);
    DeleteObject(bmp);
    DeleteDC(mem);
}

LRESULT CALLBACK PreviewHandler::Impl::Proc(HWND hwnd, UINT msg, WPARAM wParam,
                                            LPARAM lParam) {
    Impl* self = reinterpret_cast<Impl*>(GetWindowLongPtrW(hwnd, GWLP_USERDATA));

    // Creation happens before USERDATA is set, so the message is handled before
    // looking the object up.
    if (!self) return DefWindowProcW(hwnd, msg, wParam, lParam);

    switch (msg) {
        case WM_PAINT: {
            PAINTSTRUCT ps = {};
            HDC dc = BeginPaint(hwnd, &ps);
            RECT rc = {};
            GetClientRect(hwnd, &rc);
            self->Paint(dc, rc);
            EndPaint(hwnd, &ps);
            return 0;
        }
        case WM_SIZE:
            self->clientWidth = LOWORD(lParam);
            self->clientHeight = HIWORD(lParam);
            self->Relayout();
            self->ScrollTo(self->scrollY);
            InvalidateRect(hwnd, nullptr, FALSE);
            return 0;
        case WM_MOUSEWHEEL: {
            int delta = GET_WHEEL_DELTA_WPARAM(wParam);
            self->ScrollTo(self->scrollY - (delta / WHEEL_DELTA) * Scale(60, self->dpi));
            return 0;
        }
        case WM_LBUTTONDOWN: {
            if (!self->loaded) return 0;
            ::SetFocus(hwnd);
            int x = GET_X_LPARAM(lParam);
            int y = GET_Y_LPARAM(lParam) + self->scrollY - Scale(kHeaderHeight, self->dpi);
            if (y < 0) return 0;
            int gap = Scale(kGap, self->dpi);
            int step = self->tileHeight + gap;
            int row = y / step;
            int col = (x - gap) / (self->tileWidth + gap);
            if (col < 0 || col >= self->columns) return 0;
            int index = row * self->columns + col;
            if (index >= 0 && index < static_cast<int>(self->zipped.Count())) {
                self->selected = index;
                InvalidateRect(hwnd, nullptr, FALSE);
            }
            return 0;
        }
        case WM_KEYDOWN: {
            if (!self->loaded) return 0;
            switch (wParam) {
                case VK_LEFT: self->MoveSelection(-1); break;
                case VK_RIGHT: self->MoveSelection(1); break;
                case VK_UP: self->MoveSelection(-self->columns); break;
                case VK_DOWN: self->MoveSelection(self->columns); break;
                case VK_HOME: self->MoveSelection(-static_cast<int>(self->zipped.Count())); break;
                case VK_END: self->MoveSelection(static_cast<int>(self->zipped.Count())); break;
                case VK_PRIOR: self->ScrollTo(self->scrollY - self->clientHeight); break;
                case VK_NEXT: self->ScrollTo(self->scrollY + self->clientHeight); break;
                default: break;
            }
            return 0;
        }
        case WM_SETFOCUS:
            self->focused = true;
            InvalidateRect(hwnd, nullptr, FALSE);
            return 0;
        case WM_KILLFOCUS:
            self->focused = false;
            InvalidateRect(hwnd, nullptr, FALSE);
            return 0;
        case WM_ERASEBKGND:
            return 1;
        default:
            return DefWindowProcW(hwnd, msg, wParam, lParam);
    }
}

namespace {

// EnsurePreviewClass registers the child window class once per process.
bool EnsurePreviewClass() {
    static bool registered = false;
    if (registered) return true;

    WNDCLASSEXW wc = {};
    wc.cbSize = sizeof(wc);
    wc.style = CS_HREDRAW | CS_VREDRAW;
    wc.lpfnWndProc = PreviewHandler::Impl::Proc;
    wc.hInstance = g_module;
    wc.hCursor = LoadCursorW(nullptr, IDC_ARROW);
    wc.lpszClassName = L"ZipScopePreviewPane";
    if (!RegisterClassExW(&wc) && GetLastError() != ERROR_CLASS_ALREADY_EXISTS) {
        return false;
    }
    registered = true;
    return true;
}

}  // namespace

// ---------------------------------------------------------------------------
// PreviewHandler
// ---------------------------------------------------------------------------

PreviewHandler::PreviewHandler()
    : ref_(1), stream_(nullptr), site_(nullptr), parent_(nullptr), hwnd_(nullptr),
      rect_{}, previewing_(false), impl_(new Impl()) {}

PreviewHandler::~PreviewHandler() {
    if (hwnd_) DestroyWindow(hwnd_);
    if (impl_) {
        if (impl_->font) DeleteObject(impl_->font);
        if (impl_->fontSmall) DeleteObject(impl_->fontSmall);
        delete impl_;
    }
    if (stream_) stream_->Release();
    if (site_) site_->Release();
}

IFACEMETHODIMP PreviewHandler::QueryInterface(REFIID riid, void** ppv) {
    if (!ppv) return E_POINTER;
    *ppv = nullptr;
    if (riid == IID_IUnknown || riid == IID_IInitializeWithStream) {
        *ppv = static_cast<IInitializeWithStream*>(this);
    }
    else if (riid == IID_IPreviewHandler) {
        *ppv = static_cast<IPreviewHandler*>(this);
    }
    else if (riid == IID_IObjectWithSite) {
        *ppv = static_cast<IObjectWithSite*>(this);
    }
    else {
        return E_NOINTERFACE;
    }
    AddRef();
    return S_OK;
}

IFACEMETHODIMP_(ULONG) PreviewHandler::AddRef() { return InterlockedIncrement(&ref_); }

IFACEMETHODIMP_(ULONG) PreviewHandler::Release() {
    LONG n = InterlockedDecrement(&ref_);
    if (n == 0) delete this;
    return n;
}

IFACEMETHODIMP PreviewHandler::Initialize(IStream* stream, DWORD) {
    if (!stream) return E_INVALIDARG;
    if (stream_) return E_UNEXPECTED;
    stream->AddRef();
    stream_ = stream;
    return S_OK;
}

IFACEMETHODIMP PreviewHandler::SetWindow(HWND hwnd, const RECT* prc) {
    if (!hwnd || !prc) return E_INVALIDARG;
    parent_ = hwnd;
    rect_ = *prc;

    if (!impl_) return E_OUTOFMEMORY;
    if (!EnsurePreviewClass()) return E_FAIL;

    if (!hwnd_) {
        hwnd_ = CreateWindowExW(0, L"ZipScopePreviewPane", L"",
                                WS_CHILD | WS_VISIBLE | WS_TABSTOP,
                                prc->left, prc->top, prc->right - prc->left,
                                prc->bottom - prc->top, hwnd, nullptr, g_module, nullptr);
        if (!hwnd_) return E_FAIL;

        SetWindowLongPtrW(hwnd_, GWLP_USERDATA, reinterpret_cast<LONG_PTR>(impl_));
        impl_->hwnd = hwnd_;

        impl_->dpi = GetDpiForWindow(hwnd_);
        if (impl_->dpi == 0) impl_->dpi = 96;

        impl_->font = CreateFontW(-Scale(13, impl_->dpi), 0, 0, 0, FW_NORMAL, FALSE, FALSE,
                                  FALSE, DEFAULT_CHARSET, OUT_DEFAULT_PRECIS,
                                  CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY,
                                  DEFAULT_PITCH | FF_DONTCARE, L"Segoe UI");
        impl_->fontSmall = CreateFontW(-Scale(11, impl_->dpi), 0, 0, 0, FW_NORMAL, FALSE,
                                       FALSE, FALSE, DEFAULT_CHARSET, OUT_DEFAULT_PRECIS,
                                       CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY,
                                       DEFAULT_PITCH | FF_DONTCARE, L"Segoe UI");

        impl_->clientWidth = prc->right - prc->left;
        impl_->clientHeight = prc->bottom - prc->top;
        impl_->Relayout();
    }
    else {
        MoveWindow(hwnd_, prc->left, prc->top, prc->right - prc->left,
                   prc->bottom - prc->top, TRUE);
    }
    ShowWindow(hwnd_, SW_SHOW);
    return S_OK;
}

IFACEMETHODIMP PreviewHandler::SetRect(const RECT* prc) {
    if (!prc) return E_INVALIDARG;
    rect_ = *prc;
    if (hwnd_) {
        MoveWindow(hwnd_, prc->left, prc->top, prc->right - prc->left,
                   prc->bottom - prc->top, TRUE);
    }
    return S_OK;
}

IFACEMETHODIMP PreviewHandler::DoPreview() {
    if (!stream_ || !impl_) return E_UNEXPECTED;
    if (previewing_) return S_OK;
    previewing_ = true;

    try {
        // Read the stream once. The bytes stay in the handler for the lifetime
        // of the preview so scrolling does not re-read the file.
        if (!ReadStreamToEnd(stream_, kMaxArchiveBytes, &impl_->bytes)) {
            impl_->error = L"この ZIP を読み込めません";
            if (hwnd_) InvalidateRect(hwnd_, nullptr, FALSE);
            return S_OK;  // A failure is shown in the pane, not returned to Explorer.
        }

        ZipLimits limits;
        limits.maxEntries = kPreviewMaxEntries;

        impl_->thumbs.clear();
        impl_->selected = 0;
        impl_->scrollY = 0;
        impl_->error.clear();
        impl_->truncated = false;
        impl_->loaded = false;

        if (!impl_->zipped.Index(impl_->bytes, limits)) {
            impl_->error = L"この ZIP には表示できる画像がありません";
        }
        else {
            impl_->loaded = true;
            impl_->truncated = impl_->zipped.Truncated();
            impl_->thumbs.resize(impl_->zipped.Count());
        }

        impl_->Relayout();
        if (hwnd_) InvalidateRect(hwnd_, nullptr, FALSE);
        return S_OK;
    }
    catch (...) {
        impl_->error = L"プレビュー中にエラーが発生しました";
        if (hwnd_) InvalidateRect(hwnd_, nullptr, FALSE);
        return S_OK;
    }
}

IFACEMETHODIMP PreviewHandler::Unload() {
    previewing_ = false;
    if (impl_) {
        // Release the archive and every decoded thumbnail. Explorer keeps the
        // handler alive between selections, so holding these would leak memory
        // proportional to the number of files previewed.
        impl_->bytes.clear();
        impl_->bytes.shrink_to_fit();
        impl_->zipped = ZipArchive();
        impl_->thumbs.clear();
        impl_->thumbs.shrink_to_fit();
        impl_->loaded = false;
        impl_->error.clear();
    }
    if (hwnd_) {
        DestroyWindow(hwnd_);
        hwnd_ = nullptr;
        if (impl_) impl_->hwnd = nullptr;
    }
    if (stream_) {
        stream_->Release();
        stream_ = nullptr;
    }
    return S_OK;
}

IFACEMETHODIMP PreviewHandler::SetFocus() {
    if (hwnd_) {
        ::SetFocus(hwnd_);
        return S_OK;
    }
    return S_FALSE;
}

IFACEMETHODIMP PreviewHandler::QueryFocus(HWND* phwnd) {
    if (!phwnd) return E_INVALIDARG;
    *phwnd = GetFocus();
    return *phwnd ? S_OK : S_FALSE;
}

IFACEMETHODIMP PreviewHandler::TranslateAccelerator(MSG* pmsg) {
    // Only claim keys the grid actually uses, so Explorer keeps Tab, Escape and
    // its own shortcuts. Returning S_FALSE passes the message on.
    if (!pmsg || !hwnd_ || GetFocus() != hwnd_) return S_FALSE;

    if (pmsg->message != WM_KEYDOWN) return S_FALSE;

    switch (pmsg->wParam) {
        case VK_LEFT:
        case VK_RIGHT:
        case VK_UP:
        case VK_DOWN:
        case VK_HOME:
        case VK_END:
        case VK_PRIOR:
        case VK_NEXT:
            SendMessageW(hwnd_, WM_KEYDOWN, pmsg->wParam, pmsg->lParam);
            return S_OK;
        default:
            return S_FALSE;
    }
}

IFACEMETHODIMP PreviewHandler::SetSite(IUnknown* punkSite) {
    if (site_) {
        site_->Release();
        site_ = nullptr;
    }
    if (punkSite) {
        punkSite->AddRef();
        site_ = punkSite;
    }
    return S_OK;
}

IFACEMETHODIMP PreviewHandler::GetSite(REFIID riid, void** ppv) {
    if (!ppv) return E_POINTER;
    *ppv = nullptr;
    if (!site_) return E_FAIL;
    return site_->QueryInterface(riid, ppv);
}

}  // namespace zipscope
