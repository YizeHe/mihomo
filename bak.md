# Backup Record

Date: 2026-07-02

## Backed Up Files

| Original Path | Backup Name | Description |
|---|---|---|
| `tangent-vpn-backend/main.go` | `bak/backend_main.go` | Backend server entry point |
| `tangent-vpn-backend/db.go` | `bak/backend_db.go` | Database operations |
| `tangent-vpn-backend/admin.html` | `bak/backend_admin.html` | Admin panel HTML |
| `gui.go` | `bak/gui.go` | Client GUI (fyne) |

## Changes Made

- Added activation code (卡密) system
- Server reads `year.keytxt` and `month.keytxt` for activation codes
- Client initial screen changed from login to activation code input
- Original login/register moved to "Switch" button
