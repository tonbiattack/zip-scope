// CLSIDs and shared constants for the ZipScope shell extension.
#pragma once

#include <windows.h>
#include <initguid.h>

// {6F1B2A10-8C3D-4E57-9A21-5B7C0D3E8F41}
DEFINE_GUID(CLSID_ZipScopeThumbnail,
            0x6f1b2a10, 0x8c3d, 0x4e57, 0x9a, 0x21, 0x5b, 0x7c, 0x0d, 0x3e, 0x8f, 0x41);

// {6F1B2A11-8C3D-4E57-9A21-5B7C0D3E8F41}
DEFINE_GUID(CLSID_ZipScopeContextMenu,
            0x6f1b2a11, 0x8c3d, 0x4e57, 0x9a, 0x21, 0x5b, 0x7c, 0x0d, 0x3e, 0x8f, 0x41);

namespace zipscope {

// Limits mirror the application's defaults so the extension can never do more
// work than the viewer would. Explorer runs this in-process, so the bounds are
// deliberately tighter than the app's.
constexpr size_t kMaxArchiveBytes = 512ull * 1024 * 1024;  // 512 MiB
constexpr size_t kMaxEntryBytes = 128ull * 1024 * 1024;    // 128 MiB
constexpr size_t kMaxPixels = 40ull * 1000 * 1000;         // 40 MP

// kMaxThumbnailSide caps the requested thumbnail so a huge request cannot make
// the extension allocate an enormous bitmap.
constexpr UINT kMaxThumbnailSide = 1024;

}  // namespace zipscope
