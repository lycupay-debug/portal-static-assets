const DEFAULT_WIDTH = 1440;
const DEFAULT_HEIGHT = 900;

function readViewportDimension(value, fallback, maximum) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return fallback;
  return Math.round(Math.min(maximum, Math.max(240, parsed)));
}

/**
 * Resolve a shared responsive policy from browser viewport dimensions.
 * The browser remains responsible for rendering; this module centralizes
 * the profile and fluid CSS tokens so every page uses the same policy.
 */
export function resolveResponsiveLayout(searchParams) {
  const width = readViewportDimension(searchParams.get('width'), DEFAULT_WIDTH, 10000);
  const height = readViewportDimension(searchParams.get('height'), DEFAULT_HEIGHT, 10000);
  let profile;
  let gutterX;
  let contentGutter;
  let sidebarTrack;

  if (width <= 380) {
    profile = 'compact-mobile';
    gutterX = '3vw';
    contentGutter = '3vw';
    sidebarTrack = '100%';
  } else if (width <= 760) {
    profile = 'mobile';
    gutterX = '4vw';
    contentGutter = '4vw';
    sidebarTrack = '100%';
  } else if (width <= 1100) {
    profile = 'tablet';
    gutterX = '3.2vw';
    contentGutter = '2.6vw';
    sidebarTrack = '21vw';
  } else if (width >= 1920) {
    profile = 'ultrawide';
    gutterX = '4vw';
    contentGutter = '2.5vw';
    sidebarTrack = '15vw';
  } else {
    profile = 'desktop';
    gutterX = 'clamp(2.5vw, 4.2vw, 6.5vw)';
    contentGutter = 'clamp(2vw, 3vw, 4.5vw)';
    sidebarTrack = '18vw';
  }

  const gutterY = height <= 600 ? '2vh' : width <= 760 ? '3vh' : 'clamp(2vh, 4vh, 5.5vh)';
  return {
    version: 1,
    viewport: { width, height },
    profile,
    cssVariables: {
      '--layout-gutter-x': gutterX,
      '--layout-gutter-y': gutterY,
      '--layout-content-gutter': contentGutter,
      '--layout-sidebar-track': sidebarTrack,
      '--layout-table-top-gap': '8vh',
    },
  };
}
