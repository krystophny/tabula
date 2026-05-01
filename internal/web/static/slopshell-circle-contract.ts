export type SlopshellCircleCorner = 'top_left' | 'top_right' | 'bottom_left' | 'bottom_right';
export type SlopshellCircleSegmentKind = 'session' | 'toggle' | 'tool';
export type SlopshellCircleSegmentID =
  | 'dialogue'
  | 'meeting'
  | 'silent'
  | 'fast'
  | 'prompt'
  | 'text_note'
  | 'pointer'
  | 'highlight'
  | 'ink';

type SlopshellCircleSegmentContract = {
  id: SlopshellCircleSegmentID,
  kind: SlopshellCircleSegmentKind,
  icon: string,
  icon_id: string,
  label: string,
  angle_deg: number,
  radius_px: number,
};

type SlopshellCircleCornerOption = {
  id: SlopshellCircleCorner,
  icon: string,
  label: string,
};

function strokeIcon(paths: string, fillRule = '') {
  const fill = fillRule ? ` fill-rule="${fillRule}"` : '';
  return `<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"${fill}>${paths}</svg>`;
}

function filledIcon(paths: string) {
  return `<svg viewBox="0 0 24 24" aria-hidden="true" fill="currentColor">${paths}</svg>`;
}

export const SLOPSHELL_CIRCLE_TOOL_ICON_IDS: Record<string, string> = {
  pointer: 'arrow',
  highlight: 'marker',
  ink: 'pen_nib',
  text_note: 'sticky_note',
  prompt: 'mic',
};

export const SLOPSHELL_CIRCLE_TOOL_ICONS: Record<string, string> = {
  pointer: strokeIcon('<path d="m6 4 10 8-4.5 1.3L15 20l-2.4 1-3.4-6.8L6 17Z"/>'),
  highlight: strokeIcon('<path d="m5 15 4 4"/><path d="m8 18 8.5-8.5a2.1 2.1 0 0 0-3-3L5 15v4h4Z"/><path d="M13 7l4 4"/><path d="M4 21h16"/>'),
  ink: strokeIcon('<path d="m4 20 4.5-1 9-9a2.1 2.1 0 0 0-3-3l-9 9Z"/><path d="m13 7 4 4"/><path d="M4 20h5"/>'),
  text_note: strokeIcon('<rect x="3" y="6" width="18" height="12" rx="2"/><path d="M6 10h.01"/><path d="M9 10h.01"/><path d="M12 10h.01"/><path d="M15 10h.01"/><path d="M18 10h.01"/><path d="M6 14h12"/>'),
  prompt: strokeIcon('<path d="M12 4v8"/><path d="M8.5 8.5a3.5 3.5 0 0 1 7 0V12a3.5 3.5 0 0 1-7 0Z"/><path d="M6 11.5a6 6 0 0 0 12 0"/><path d="M12 17.5V21"/><path d="M9 21h6"/>'),
};

export const SLOPSHELL_CIRCLE_SEGMENTS: SlopshellCircleSegmentContract[] = [
  {
    id: 'dialogue',
    kind: 'session',
    icon_id: 'dialogue',
    label: 'Live mode: Dialogue',
    angle_deg: 18,
    radius_px: 78,
    icon: strokeIcon('<path d="M4.5 8a3.5 3.5 0 0 1 3.5-3.5h5A3.5 3.5 0 0 1 16.5 8v2A3.5 3.5 0 0 1 13 13.5H9.5L6 16v-2.5A3.5 3.5 0 0 1 4.5 10Z"/><path d="M13.5 10.5H16a3 3 0 0 1 3 3V15a3 3 0 0 1-3 3h-2.5L10.5 20v-2"/>'),
  },
  {
    id: 'meeting',
    kind: 'session',
    icon_id: 'meeting',
    label: 'Live mode: Meeting',
    angle_deg: 7,
    radius_px: 148,
    icon: strokeIcon('<path d="M8 12a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5Z"/><path d="M16.5 11a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z"/><path d="M4.5 18a3.5 3.5 0 0 1 7 0"/><path d="M13.5 18a3 3 0 0 1 6 0"/>'),
  },
  {
    id: 'silent',
    kind: 'toggle',
    icon_id: 'silent',
    label: 'Output mode: Silent',
    angle_deg: 30,
    radius_px: 232,
    icon: strokeIcon('<path d="M5 10h4l5-4v12l-5-4H5Z"/><path d="m17 9 4 6"/><path d="m21 9-4 6"/>'),
  },
  {
    id: 'fast',
    kind: 'toggle',
    icon_id: 'fast',
    label: 'Turn mode: Fast',
    angle_deg: 38,
    radius_px: 286,
    icon: strokeIcon('<path d="M13 3 6 13h4l-1 8 9-12h-4l1-6Z"/>'),
  },
  {
    id: 'prompt',
    kind: 'tool',
    icon_id: 'mic',
    label: 'Tool: Prompt',
    angle_deg: 66,
    radius_px: 92,
    icon: SLOPSHELL_CIRCLE_TOOL_ICONS.prompt,
  },
  {
    id: 'text_note',
    kind: 'tool',
    icon_id: 'sticky_note',
    label: 'Tool: Text note',
    angle_deg: 82,
    radius_px: 148,
    icon: SLOPSHELL_CIRCLE_TOOL_ICONS.text_note,
  },
  {
    id: 'pointer',
    kind: 'tool',
    icon_id: 'arrow',
    label: 'Tool: Pointer',
    angle_deg: 89,
    radius_px: 217,
    icon: SLOPSHELL_CIRCLE_TOOL_ICONS.pointer,
  },
  {
    id: 'highlight',
    kind: 'tool',
    icon_id: 'marker',
    label: 'Tool: Highlight',
    angle_deg: 69,
    radius_px: 228,
    icon: SLOPSHELL_CIRCLE_TOOL_ICONS.highlight,
  },
  {
    id: 'ink',
    kind: 'tool',
    icon_id: 'pen_nib',
    label: 'Tool: Ink',
    angle_deg: 52,
    radius_px: 238,
    icon: SLOPSHELL_CIRCLE_TOOL_ICONS.ink,
  },
];

export const SLOPSHELL_CIRCLE_CORNERS: SlopshellCircleCornerOption[] = [
  {
    id: 'top_left',
    label: 'Place Slopshell Circle in the top left corner',
    icon: filledIcon('<rect x="4" y="4" width="16" height="16" rx="3" opacity="0.28"/><path d="M4 4h8a0 0 0 0 1 0 0v8a0 0 0 0 1 0 0H7a3 3 0 0 1-3-3V4a0 0 0 0 1 0 0Z"/>'),
  },
  {
    id: 'top_right',
    label: 'Place Slopshell Circle in the top right corner',
    icon: filledIcon('<rect x="4" y="4" width="16" height="16" rx="3" opacity="0.28"/><path d="M12 4h8a0 0 0 0 1 0 0v5a3 3 0 0 1-3 3h-5a0 0 0 0 1 0 0V4a0 0 0 0 1 0 0Z"/>'),
  },
  {
    id: 'bottom_left',
    label: 'Place Slopshell Circle in the bottom left corner',
    icon: filledIcon('<rect x="4" y="4" width="16" height="16" rx="3" opacity="0.28"/><path d="M4 12h8a0 0 0 0 1 0 0v8a0 0 0 0 1 0 0H7a3 3 0 0 1-3-3v-5a0 0 0 0 1 0 0Z"/>'),
  },
  {
    id: 'bottom_right',
    label: 'Place Slopshell Circle in the bottom right corner',
    icon: filledIcon('<rect x="4" y="4" width="16" height="16" rx="3" opacity="0.28"/><path d="M12 12h8a0 0 0 0 1 0 0v5a3 3 0 0 1-3 3h-5a0 0 0 0 1 0 0v-8a0 0 0 0 1 0 0Z"/>'),
  },
];

export const SLOPSHELL_CIRCLE_BUG_ICON = strokeIcon('<path d="M12 8.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7Z"/><path d="M12 4v2.2"/><path d="M6.5 6.5 8 8"/><path d="M17.5 6.5 16 8"/><path d="M5 11h2"/><path d="M17 11h2"/><path d="M6.5 15.5 8 14"/><path d="M17.5 15.5 16 14"/><path d="M9.4 18h5.2"/>');

export const SLOPSHELL_CIRCLE_LAYOUT = Object.freeze({
  storage_key: 'slopshell.toolPalettePosition',
  bug_report_entry: 'top_panel_overflow',
  shell_size_px: 312,
  shell_size_mobile_px: 292,
  dot_size_px: 58,
  dot_size_mobile_px: 56,
  segment_size_px: 56,
  segment_size_mobile_px: 54,
  viewport_margin_px: 18,
  viewport_margin_mobile_px: 14,
  corners: SLOPSHELL_CIRCLE_CORNERS.map((corner) => corner.id),
  segments: SLOPSHELL_CIRCLE_SEGMENTS.map((segment) => ({
    id: segment.id,
    kind: segment.kind,
    icon_id: segment.icon_id,
    label: segment.label,
    angle_deg: segment.angle_deg,
    radius_px: segment.radius_px,
  })),
});

export function normalizeSlopshellCircleCorner(value: string): SlopshellCircleCorner {
  const clean = String(value || '').trim().toLowerCase();
  return SLOPSHELL_CIRCLE_CORNERS.some((corner) => corner.id === clean)
    ? clean as SlopshellCircleCorner
    : 'bottom_right';
}

export function slopshellCircleToolIcon(tool: string) {
  const clean = String(tool || '').trim().toLowerCase();
  return SLOPSHELL_CIRCLE_TOOL_ICONS[clean] || SLOPSHELL_CIRCLE_TOOL_ICONS.pointer;
}

export function slopshellCircleToolIconID(tool: string) {
  const clean = String(tool || '').trim().toLowerCase();
  return SLOPSHELL_CIRCLE_TOOL_ICON_IDS[clean] || SLOPSHELL_CIRCLE_TOOL_ICON_IDS.pointer;
}
