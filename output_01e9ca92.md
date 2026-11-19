# UI Alignment: Added Standardized Form Error Component

## Summary of Changes
- Created a new reusable `FormFieldError` component in `src/components/common/FormFieldError.jsx` to standardize the display of inline form validation errors (consistent with TSenta UI principles).
- Refactored `AddRoleModal.jsx` to use this new `FormFieldError` component instead of relying solely on toast notifications for form validation. This provides immediate, accessible, and explicit visual feedback to the user when form submission validation fails.

## Files Created/Modified
- Created: `oneRole_UI/src/components/common/FormFieldError.jsx`
- Modified: `oneRole_UI/src/components/modals/AddRoleModal.jsx`
