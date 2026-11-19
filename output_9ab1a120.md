# Worker Contribution: 9ab1a120

## Summary
Improved the `FilterBar` component in the `oneRole_UI` to provide better visual feedback during job reloading.

## Changes
- Updated `FilterBar` to consume the `loading` state from the `AppContext`.
- Added `disabled={loading}` to all filter buttons, workplace toggles, and inputs.
- Applied CSS styles (`opacity-50 cursor-not-allowed`) to indicate the disabled state when jobs are being reloaded, preventing redundant requests and improving UI consistency.

## Rationale
Ensuring the user knows when the system is processing filter changes, and preventing them from triggering multiple concurrent requests by disabling inputs while a reload is in progress, significantly improves the user experience and aligns the UI with modern design patterns.
