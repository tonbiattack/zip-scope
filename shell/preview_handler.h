// Preview handler interfaces for ZipScope.
//
// Explorer hosts this in-process and hands it a rectangle inside the preview
// pane. The handler implements the interfaces Explorer requires of a desktop
// (non-UI-island) preview handler:
//
//   IInitializeWithStream  - receives the file contents
//   IPreviewHandler        - SetWindow / SetRect / DoPreview / Unload
//   IObjectWithSite        - lets the host pass its IUnknown, which is how the
//                            handler tells Explorer it took focus
//
// It deliberately does not implement IInitializeWithFile: Explorer may pass
// either, and stream is the one that works for files with no path (library
// items, search results).
#pragma once

#include <windows.h>
#include <objidl.h>
#include <shobjidl.h>
#include <shtypes.h>

#include <cstdint>
#include <vector>

#include "zip_reader.h"

namespace zipscope {

// PreviewHandler is the COM object Explorer creates.
class PreviewHandler final : public IInitializeWithStream,
                             public IPreviewHandler,
                             public IObjectWithSite {
public:
    PreviewHandler();

    // IUnknown
    IFACEMETHODIMP QueryInterface(REFIID riid, void** ppv) override;
    IFACEMETHODIMP_(ULONG) AddRef() override;
    IFACEMETHODIMP_(ULONG) Release() override;

    // IInitializeWithStream
    IFACEMETHODIMP Initialize(IStream* stream, DWORD grfMode) override;

    // IPreviewHandler
    IFACEMETHODIMP SetWindow(HWND hwnd, const RECT* prc) override;
    IFACEMETHODIMP SetRect(const RECT* prc) override;
    IFACEMETHODIMP DoPreview() override;
    IFACEMETHODIMP Unload() override;
    IFACEMETHODIMP SetFocus() override;
    IFACEMETHODIMP QueryFocus(HWND* phwnd) override;
    IFACEMETHODIMP TranslateAccelerator(MSG* pmsg) override;

    // IObjectWithSite
    IFACEMETHODIMP SetSite(IUnknown* punkSite) override;
    IFACEMETHODIMP GetSite(REFIID riid, void** ppv) override;

private:
    ~PreviewHandler();

public:
    // Impl is the child window that paints the grid. It is defined in the .cpp
    // and is public because the window procedure is a free function that needs
    // to name its type.
    struct Impl;

private:
    LONG ref_;
    IStream* stream_;
    IUnknown* site_;
    HWND parent_;
    HWND hwnd_;
    RECT rect_;
    bool previewing_;
    Impl* impl_;
};

}  // namespace zipscope
