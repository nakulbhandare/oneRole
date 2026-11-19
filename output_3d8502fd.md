# UI Alignment: OneRole vs TSenta Improvements

## Summary of Improvements
1.  **Explicit Form Errors**: Implemented a standardized `FormError` component and integrated it into `AddRoleModal` for inline error feedback, moving away from relying solely on toast notifications for form validation.
2.  **Contrast Check**: Reviewed and updated the sidebar muted text color variable (`--sidebar-muted`) to improve accessibility against the dark background.
3.  **Animation Consistency**: Introduced standardized CSS transition and animation variables (`--transition-normal`, `--animation-fade`) and applied them to the sidebar and view-panel components to ensure consistent motion design across the UI.

## Implementation Details
-   **FormError Component**: Updated `AddRoleModal` to use `FormError` for inline validation.
-   **Styles**: Updated `oneRole_UI/public/styles.css` to define and use CSS variables for colors, transitions, and animations, and improved sidebar contrast.
