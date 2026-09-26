import Box from '@mui/material/Box';
import { Fragment } from 'react';
import { Mono } from './common';
import type { PathPart } from './model';

/** Between two paths (a typographic separator, not text). */
const SEPARATOR = ' · ';

/**
 * Paths of a route or tunnel, one after another: words in the UI language, technical tokens (addresses, interface
 * names, label stacks) as isolated left-to-right runs, so an RTL line reads in the right order.
 */
export function PathsView({ paths }: { paths: PathPart[][] }) {
  return (
    <Box component="span">
      {paths.map((parts, i) => (
        <Fragment key={i}>
          {i > 0 && SEPARATOR}
          {parts.map((p, j) => (
            <Fragment key={j}>
              {j > 0 && ' '}
              {p.mono ? <Mono>{p.text}</Mono> : p.text}
            </Fragment>
          ))}
        </Fragment>
      ))}
    </Box>
  );
}
