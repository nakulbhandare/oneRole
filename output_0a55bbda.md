# Worker 0a55bbda: UI Alignment Contribution

## Summary
Refactored `MatchCard.jsx` to align theme colors and button styling with the TSenta design principles as documented in `styles.css`. This improves the visual consistency of the job match cards. Retained the `btn-card-apply` class to ensure continued compatibility with existing tests.

## Missing/Improvement Items Addressed
- **Inconsistent button styling**: Standardized 'View', 'Pass', and 'Apply' buttons using consistent Tailwind classes.
- **Theme Color Mismatch**: Updated card pastel theme colors to match the exact values defined in the TSenta design system (`styles.css`).
- **Button Specificity**: Removed redundant conflicting Tailwind classes from button elements, relying instead on a cleaner, unified Tailwind approach.

## Files Modified
- `/Users/nakulbhandare/go/src/github.com/oneRole/oneRole_UI/src/components/dashboard/MatchCard.jsx`
