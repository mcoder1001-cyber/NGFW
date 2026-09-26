import Link from '@mui/material/Link';
import { useLayoutEffect, useRef, type KeyboardEvent, type MouseEvent, type ReactNode } from 'react';

export interface KeyButtonProps {
  children: ReactNode;
  onClick: () => void;
  /** Accessible name when the visible key alone is not enough (`Open interface host-w1l0`). */
  'aria-label'?: string | undefined;
  /** From the grid's `renderCell` params: -1 unless the cell is the grid's focus target (roving tabindex). */
  tabIndex?: number | undefined;
  /** From the grid's `renderCell` params: the cell has keyboard focus → the button takes it. */
  hasFocus?: boolean | undefined;
  disabled?: boolean | undefined;
}

/**
 * The key cell of a config list as a real `<button>` (A11Y-1): reachable with Tab and the grid's arrow keys, opened with
 * Enter or Space, announced with its name. The row click of P08 stays a mouse convenience; the keyboard path is this
 * button. Keys are identifiers (interface names, refs): rendered left-to-right in monospace inside RTL text.
 */
export function KeyButton({ children, onClick, tabIndex, hasFocus, disabled, 'aria-label': ariaLabel }: KeyButtonProps) {
  const ref = useRef<HTMLButtonElement>(null);
  // MUI X moves focus between cells; when it lands on this cell the button inside takes it (as GridActionsCell does)
  useLayoutEffect(() => {
    if (hasFocus) ref.current?.focus();
  }, [hasFocus]);
  return (
    <Link
      ref={ref}
      component="button"
      type="button"
      underline="hover"
      disabled={disabled}
      tabIndex={tabIndex}
      aria-label={ariaLabel}
      onClick={(e: MouseEvent) => {
        e.stopPropagation(); // the row click would open the same item a second time
        onClick();
      }}
      onKeyDown={(e: KeyboardEvent) => {
        // the grid must not treat Enter/Space as row selection or cell editing: they activate this button
        if (e.key === 'Enter' || e.key === ' ') e.stopPropagation();
      }}
      sx={{ fontFamily: (th) => th.vrx.monoFontFamily, textAlign: 'start', verticalAlign: 'middle', maxInlineSize: '100%', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
    >
      <bdi dir="ltr">{children}</bdi>
    </Link>
  );
}
