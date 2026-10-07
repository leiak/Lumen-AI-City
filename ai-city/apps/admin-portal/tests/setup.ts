// Vitest setup — extend expect with @testing-library/jest-dom matchers
// (toBeInTheDocument, toHaveAttribute, etc.)
// Also auto-cleanup the rendered DOM after each test so findBy* queries
// don't see leftover nodes from earlier `render()` calls (this matters for
// Header which has a long-lived fetch promise).
import '@testing-library/jest-dom/vitest';
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

afterEach(() => {
  cleanup();
});
