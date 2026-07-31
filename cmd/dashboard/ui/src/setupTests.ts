// Runs once before the test suite. Registers the jest-dom matchers
// (toBeInTheDocument, etc.) on vitest's `expect`, and makes sure each test
// starts from a clean DOM by unmounting whatever the previous test rendered
// (vitest globals are intentionally off, so this can't rely on an implicit
// afterEach — it registers its own via the `vitest` import below).
import '@testing-library/jest-dom/vitest';
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

afterEach(() => {
  cleanup();
});
