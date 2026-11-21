// ZipScope Shell Extension
//
// An in-process COM server that gives Windows Explorer two things:
//
//   1. A thumbnail provider, so a ZIP/CBZ shows its first image as the file
//      icon instead of a generic archive icon.
//   2. A context menu entry, so a ZIP can be opened in ZipScope directly.
//
// The extension is deliberately small and defensive: Explorer loads it into
// explorer.exe, so a crash or a hang takes the desktop with it. Every entry
// point is wrapped in try/catch, every COM call is checked, and all work is
// bounded by the same limits the main application uses.

#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <shellapi.h>
#include <shlobj.h>
#include <shobjidl.h>
#include <thumbcache.h>
#include <shlwapi.h>
#include <propkey.h>
#include <new>
#include <string>
#include <vector>

#include "zip_reader.h"
#include "decode.h"
#include "guids.h"
#include "dllmain.h"

#pragma comment(lib, "shlwapi.lib")
#pragma comment(lib, "windowscodecs.lib")
#pragma comment(lib, "ole32.lib")
#pragma comment(lib, "uuid.lib")

namespace zipscope {

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

// IsArchivePath reports whether path has a ZIP/CBZ extension.
bool IsArchivePath(const wchar_t* path) {
    if (!path) return false;
    const wchar_t* ext = PathFindExtensionW(path);
    if (!ext) return false;
    return _wcsicmp(ext, L".zip") == 0 || _wcsicmp(ext, L".cbz") == 0;
}

// ---------------------------------------------------------------------------
// Thumbnail provider
// ---------------------------------------------------------------------------

// ThumbnailProvider implements IThumbnailProvider plus the initialisation
// interfaces Explorer uses to hand it a file.
class ThumbnailProvider final : public IInitializeWithStream,
                                public IThumbnailProvider {
public:
    ThumbnailProvider() : ref_(1), stream_(nullptr) {}

    // IUnknown
    IFACEMETHODIMP QueryInterface(REFIID riid, void** ppv) override {
        if (!ppv) return E_POINTER;
        *ppv = nullptr;
        if (riid == IID_IUnknown || riid == IID_IInitializeWithStream) {
            *ppv = static_cast<IInitializeWithStream*>(this);
        } else if (riid == IID_IThumbnailProvider) {
            *ppv = static_cast<IThumbnailProvider*>(this);
        } else {
            return E_NOINTERFACE;
        }
        AddRef();
        return S_OK;
    }

    IFACEMETHODIMP_(ULONG) AddRef() override {
        return InterlockedIncrement(&ref_);
    }

    IFACEMETHODIMP_(ULONG) Release() override {
        LONG n = InterlockedDecrement(&ref_);
        if (n == 0) delete this;
        return n;
    }

    // IInitializeWithStream
    IFACEMETHODIMP Initialize(IStream* stream, DWORD /*grfMode*/) override {
        if (!stream) return E_INVALIDARG;
        if (stream_) return E_UNEXPECTED;
        stream->AddRef();
        stream_ = stream;
        return S_OK;
    }

    // IThumbnailProvider
    IFACEMETHODIMP GetThumbnail(UINT cx, HBITMAP* phbmp,
                                WTS_ALPHATYPE* pdwAlpha) override {
        if (!phbmp || !pdwAlpha) return E_POINTER;
        *phbmp = nullptr;
        *pdwAlpha = WTSAT_UNKNOWN;
        if (!stream_) return E_UNEXPECTED;

        try {
            // Read the whole archive into memory. Explorer hands us a stream,
            // not a path, so the ZIP reader works on a buffer.
            std::vector<BYTE> archive;
            if (!ReadStreamToEnd(stream_, kMaxArchiveBytes, &archive)) {
                return E_FAIL;
            }

            // Find the first image entry in natural order, which is the same
            // representative image the application shows.
            std::vector<BYTE> image;
            if (!ReadFirstImage(archive, kMaxEntryBytes, kMaxPixels, &image)) {
                return E_FAIL;
            }

            // Decode and scale with WIC, which handles JPEG/PNG/GIF/WebP.
            HBITMAP bmp = nullptr;
            if (!DecodeToHBITMAP(image, cx, &bmp)) {
                return E_FAIL;
            }
            *phbmp = bmp;
            *pdwAlpha = WTSAT_ARGB;
            return S_OK;
        } catch (...) {
            // Never let an exception cross the COM boundary.
            return E_FAIL;
        }
    }

private:
    ~ThumbnailProvider() {
        if (stream_) stream_->Release();
    }

    LONG ref_;
    IStream* stream_;
};

// ---------------------------------------------------------------------------
// Context menu
// ---------------------------------------------------------------------------

// ContextMenu implements the classic IContextMenu so "ZipScopeで開く" appears
// on the right-click menu of a ZIP/CBZ.
class ContextMenu final : public IShellExtInit, public IContextMenu {
public:
    ContextMenu() : ref_(1) {}

    // IUnknown
    IFACEMETHODIMP QueryInterface(REFIID riid, void** ppv) override {
        if (!ppv) return E_POINTER;
        *ppv = nullptr;
        if (riid == IID_IUnknown || riid == IID_IShellExtInit) {
            *ppv = static_cast<IShellExtInit*>(this);
        } else if (riid == IID_IContextMenu) {
            *ppv = static_cast<IContextMenu*>(this);
        } else {
            return E_NOINTERFACE;
        }
        AddRef();
        return S_OK;
    }

    IFACEMETHODIMP_(ULONG) AddRef() override {
        return InterlockedIncrement(&ref_);
    }

    IFACEMETHODIMP_(ULONG) Release() override {
        LONG n = InterlockedDecrement(&ref_);
        if (n == 0) delete this;
        return n;
    }

    // IShellExtInit
    IFACEMETHODIMP Initialize(PCIDLIST_ABSOLUTE /*pidlFolder*/,
                              IDataObject* pdtobj, HKEY /*hkeyProgID*/) override {
        if (!pdtobj) return E_INVALIDARG;
        path_.clear();

        FORMATETC fmt = {CF_HDROP, nullptr, DVASPECT_CONTENT, -1, TYMED_HGLOBAL};
        STGMEDIUM stg = {};
        if (FAILED(pdtobj->GetData(&fmt, &stg))) return E_FAIL;

        HDROP drop = static_cast<HDROP>(GlobalLock(stg.hGlobal));
        if (!drop) {
            ReleaseStgMedium(&stg);
            return E_FAIL;
        }

        // Only single-selection is supported: acting on a whole selection would
        // need a confirmation UI the extension does not have.
        UINT count = DragQueryFileW(drop, 0xFFFFFFFF, nullptr, 0);
        if (count == 1) {
            wchar_t buf[MAX_PATH * 2] = {};
            if (DragQueryFileW(drop, 0, buf, ARRAYSIZE(buf)) > 0) {
                if (IsArchivePath(buf)) path_ = buf;
            }
        }
        GlobalUnlock(stg.hGlobal);
        ReleaseStgMedium(&stg);

        return path_.empty() ? E_FAIL : S_OK;
    }

    // IContextMenu
    IFACEMETHODIMP QueryContextMenu(HMENU hmenu, UINT indexMenu, UINT idCmdFirst,
                                    UINT /*idCmdLast*/, UINT uFlags) override {
        if (uFlags & CMF_DEFAULTONLY) return MAKE_HRESULT(SEVERITY_SUCCESS, 0, 0);
        if (path_.empty()) return MAKE_HRESULT(SEVERITY_SUCCESS, 0, 0);

        // Insert after "Open" so the entry sits with the other open actions.
        InsertMenuW(hmenu, indexMenu, MF_BYPOSITION | MF_STRING, idCmdFirst + kCmdOpen,
                    L"ZipScopeで開く");
        return MAKE_HRESULT(SEVERITY_SUCCESS, 0, kCmdCount);
    }

    IFACEMETHODIMP InvokeCommand(CMINVOKECOMMANDINFO* pici) override {
        if (!pici) return E_INVALIDARG;
        // A verb string means Explorer is asking for a named command; only the
        // numeric id is handled here.
        if (HIWORD(pici->lpVerb) != 0) return E_FAIL;
        if (LOWORD(pici->lpVerb) != kCmdOpen) return E_FAIL;
        if (path_.empty()) return E_FAIL;

        return LaunchZipScope(path_) ? S_OK : E_FAIL;
    }

    IFACEMETHODIMP GetCommandString(UINT_PTR idCmd, UINT uType, UINT* /*res*/,
                                    LPSTR pszName, UINT cchMax) override {
        if (idCmd != kCmdOpen) return E_FAIL;
        const wchar_t* text = L"ZipScopeで開く";
        if (uType == GCS_HELPTEXTW) {
            wcsncpy_s(reinterpret_cast<wchar_t*>(pszName), cchMax, text, _TRUNCATE);
            return S_OK;
        }
        if (uType == GCS_VERBW) {
            wcsncpy_s(reinterpret_cast<wchar_t*>(pszName), cchMax, L"ZipScopeOpen", _TRUNCATE);
            return S_OK;
        }
        if (uType == GCS_HELPTEXTA || uType == GCS_VERBA) {
            WideCharToMultiByte(CP_ACP, 0, text, -1, pszName, cchMax, nullptr, nullptr);
            return S_OK;
        }
        return E_FAIL;
    }

private:
    ~ContextMenu() = default;

    enum { kCmdOpen = 0, kCmdCount = 1 };

    LONG ref_;
    std::wstring path_;
};

// ---------------------------------------------------------------------------
// Class factory
// ---------------------------------------------------------------------------

template <typename T>
class ClassFactory final : public IClassFactory {
public:
    ClassFactory() : ref_(1) {}

    IFACEMETHODIMP QueryInterface(REFIID riid, void** ppv) override {
        if (!ppv) return E_POINTER;
        *ppv = nullptr;
        if (riid == IID_IUnknown || riid == IID_IClassFactory) {
            *ppv = static_cast<IClassFactory*>(this);
            AddRef();
            return S_OK;
        }
        return E_NOINTERFACE;
    }

    IFACEMETHODIMP_(ULONG) AddRef() override { return InterlockedIncrement(&ref_); }

    IFACEMETHODIMP_(ULONG) Release() override {
        LONG n = InterlockedDecrement(&ref_);
        if (n == 0) delete this;
        return n;
    }

    IFACEMETHODIMP CreateInstance(IUnknown* outer, REFIID riid, void** ppv) override {
        if (ppv) *ppv = nullptr;
        if (outer) return CLASS_E_NOAGGREGATION;
        T* obj = new (std::nothrow) T();
        if (!obj) return E_OUTOFMEMORY;
        HRESULT hr = obj->QueryInterface(riid, ppv);
        obj->Release();
        return hr;
    }

    IFACEMETHODIMP LockServer(BOOL lock) override {
        if (lock) DllAddRef();
        else DllRelease();
        return S_OK;
    }

private:
    ~ClassFactory() = default;
    LONG ref_;
};

}  // namespace zipscope

// ---------------------------------------------------------------------------
// DLL exports
// ---------------------------------------------------------------------------

STDAPI DllGetClassObject(REFCLSID rclsid, REFIID riid, void** ppv) {
    if (!ppv) return E_POINTER;
    *ppv = nullptr;
    try {
        if (rclsid == CLSID_ZipScopeThumbnail) {
            auto* f = new (std::nothrow) zipscope::ClassFactory<zipscope::ThumbnailProvider>();
            if (!f) return E_OUTOFMEMORY;
            HRESULT hr = f->QueryInterface(riid, ppv);
            f->Release();
            return hr;
        }
        if (rclsid == CLSID_ZipScopeContextMenu) {
            auto* f = new (std::nothrow) zipscope::ClassFactory<zipscope::ContextMenu>();
            if (!f) return E_OUTOFMEMORY;
            HRESULT hr = f->QueryInterface(riid, ppv);
            f->Release();
            return hr;
        }
    } catch (...) {
        return E_FAIL;
    }
    return CLASS_E_CLASSNOTAVAILABLE;
}

STDAPI DllCanUnloadNow() {
    return zipscope::DllRefCount() == 0 ? S_OK : S_FALSE;
}
