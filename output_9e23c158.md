# UI Alignment: OneRole vs TSenta

## Summary of Improvements
- Implemented a standardized `FormError` component for better feedback.
- Updated `AddRoleModal` to use `FormError` for explicit error handling instead of relying solely on toasts.
- Ensured that error messages are cleared when users modify form inputs, improving user experience.

## Implemented Items
1. [x] **Explicit Form Errors**: Created `FormError.jsx` and integrated it into `AddRoleModal.jsx`.

## Pending Items
- [ ] **Contrast Check**: Review sidebar text contrast against background.
- [ ] **Animation Consistency**: Standardize transitions across modals and components.
