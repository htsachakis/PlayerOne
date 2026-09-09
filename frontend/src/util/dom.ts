/** Small DOM helpers, so component code reads as structure rather than plumbing. */

type Attributes = Record<string, string | number | boolean | undefined>;

/** Creates an element with attributes and children in one call. */
export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs: Attributes = {},
  ...children: Array<Node | string | null | undefined>
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);

  for (const [key, value] of Object.entries(attrs)) {
    if (value === undefined || value === false) continue;
    if (key === 'class') node.className = String(value);
    else if (key === 'text') node.textContent = String(value);
    else if (value === true) node.setAttribute(key, '');
    else node.setAttribute(key, String(value));
  }

  for (const child of children) {
    if (child === null || child === undefined) continue;
    node.append(typeof child === 'string' ? document.createTextNode(child) : child);
  }

  return node;
}

/** Removes every child of a node. */
export function clear(node: Element): void {
  while (node.firstChild) node.removeChild(node.firstChild);
}

/** Adds or removes a class based on a condition. */
export function toggleClass(node: Element, className: string, on: boolean): void {
  node.classList.toggle(className, on);
}

/** Sets textContent only when it differs, to avoid needless layout work. */
export function setText(node: Element, text: string): void {
  if (node.textContent !== text) node.textContent = text;
}

/**
 * Highlights the parts of a string that match a query.
 *
 * Built with DOM nodes rather than an HTML string so that a title containing
 * angle brackets - common in programming tutorials - cannot be interpreted as
 * markup.
 */
export function highlight(text: string, query: string): DocumentFragment {
  const fragment = document.createDocumentFragment();
  const needle = query.trim().toLowerCase();

  if (!needle) {
    fragment.append(document.createTextNode(text));
    return fragment;
  }

  const haystack = text.toLowerCase();
  let from = 0;

  for (;;) {
    const at = haystack.indexOf(needle, from);
    if (at < 0) break;

    if (at > from) fragment.append(document.createTextNode(text.slice(from, at)));
    fragment.append(el('mark', {}, text.slice(at, at + needle.length)));
    from = at + needle.length;
  }

  if (from < text.length) fragment.append(document.createTextNode(text.slice(from)));
  return fragment;
}

/**
 * Finds the entry covering a time, using binary search.
 *
 * The transcript of a long tutorial runs to thousands of lines and this is
 * called five times a second, so a linear scan would be wasteful.
 */
export function findActiveIndex(
  entries: Array<{ start: number; end: number }>,
  position: number,
): number {
  let low = 0;
  let high = entries.length - 1;
  let found = -1;

  while (low <= high) {
    const mid = (low + high) >> 1;
    const entry = entries[mid];

    if (position < entry.start) {
      high = mid - 1;
    } else {
      // The last entry that has started is the active one. Using start alone,
      // rather than requiring position < end, keeps a line highlighted through
      // the silent gaps between captions instead of flickering off.
      found = mid;
      low = mid + 1;
    }
  }

  return found;
}
