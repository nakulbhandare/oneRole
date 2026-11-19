# UI Alignment: OneRole vs. TSenta

## Contribution
Added a "View" button to the `MatchCard` component in `oneRole_UI/src/components/dashboard/MatchCard.jsx`.

## Why this improves alignment with TSenta
The "TSenta UI" philosophy emphasizes human-in-the-loop interaction and data transparency. By providing a direct "View" link to the ATS portal within the `MatchCard`, users can quickly verify job details on the original source, strengthening the "human-in-the-loop" review process for automated applications, which is a key tenant of the TSenta model.

## Implementation Details
- Modified `/oneRole_UI/src/components/dashboard/MatchCard.jsx` to include a new "View" button in the footer.
- The button opens the `card.sourceUrl` in a new tab.
- Styled the button to be consistent with the existing "Pass" and "Apply" buttons.

## Status
Success.
