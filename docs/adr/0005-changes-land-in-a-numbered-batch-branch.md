# Changes land in a numbered Batch branch

A Change made through the served ubiq becomes a commit on one shared branch named `ubiq/<n>`, the open Batch, never
on the default branch and never on a branch per person. The server merges the default branch into the Batch on every
fetch and refuses Changes while that merge conflicts; when a Batch that holds a Change then differs from the default
branch in nothing, it is merged, and the next Change starts `ubiq/<n+1>` from the default branch. A Batch with no
Change is never taken as merged. A Maintainer merges each Batch by hand.
Committing to the default branch would need a bypass of its protection and leave the language unreviewed; a branch per
person would make every read a per-person view, since an agent must see its own Change on the next lookup. The
content-based test for "merged" survives squash and rebase merges, which an ancestor test does not.

## Consequences

- The page and every lookup show the open Batch, so readers see Changes before a Maintainer has merged them.
- Origin is the only source of truth: the server fetches before every Change, pushes straight after, and derives the
  next number from the branch names on origin, so it keeps no state of its own.
- The server never calls a forge API: opening a pull request and merging are the Maintainer's acts.
