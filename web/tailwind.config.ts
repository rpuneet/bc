import type { Config } from 'tailwindcss';
import { mycelColor } from './src/theme/mycelColor';

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      keyframes: {
        'slide-in': {
          '0%': { opacity: '0', transform: 'translateX(1rem)' },
          '100%': { opacity: '1', transform: 'translateX(0)' },
        },
        'fade-in': {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        'expand-height': {
          '0%': { opacity: '0', maxHeight: '0' },
          '100%': { opacity: '1', maxHeight: '500px' },
        },
        // Staggered mount: fade + small rise. fill-mode both holds the
        // start frame through any animation-delay so there's no flash.
        'reveal': {
          '0%': { opacity: '0', transform: 'translateY(6px)' },
          '100%': { opacity: '1', transform: 'none' },
        },
        // Indeterminate progress: a short segment sweeps left→right while a
        // streamed task runs (the stream reports no percentage).
        'indeterminate': {
          '0%': { transform: 'translateX(-100%)' },
          '100%': { transform: 'translateX(400%)' },
        },
      },
      animation: {
        'slide-in': 'slide-in 0.2s ease-out',
        'fade-in': 'fade-in 0.25s ease-out',
        'expand-height': 'expand-height 0.3s ease-out',
        'reveal': 'reveal 0.4s cubic-bezier(0.16, 1, 0.3, 1) both',
        'indeterminate': 'indeterminate 1.1s ease-in-out infinite',
      },
      colors: {
        mycel: {
          bg: mycelColor('--mycel-bg'),
          surface: mycelColor('--mycel-surface'),
          'surface-2': mycelColor('--mycel-surface-2'),
          'surface-hover': mycelColor('--mycel-surface-hover'),
          border: mycelColor('--mycel-border'),
          'border-strong': mycelColor('--mycel-border-strong'),
          text: mycelColor('--mycel-text'),
          'text-2': mycelColor('--mycel-text-2'),
          muted: mycelColor('--mycel-muted'),
          accent: mycelColor('--mycel-accent'),
          'accent-hover': mycelColor('--mycel-accent-hover'),
          'accent-subtle': mycelColor('--mycel-accent-subtle'),
          'accent-fg': mycelColor('--mycel-accent-fg'),
          success: mycelColor('--mycel-success'),
          'success-subtle': mycelColor('--mycel-success-subtle'),
          warning: mycelColor('--mycel-warning'),
          'warning-subtle': mycelColor('--mycel-warning-subtle'),
          error: mycelColor('--mycel-error'),
          'error-subtle': mycelColor('--mycel-error-subtle'),
          live: mycelColor('--mycel-live'),
          info: mycelColor('--mycel-info'),
          'info-subtle': mycelColor('--mycel-info-subtle'),
          overlay: mycelColor('--mycel-overlay'),
          'chart-1': mycelColor('--mycel-chart-1'),
          'chart-2': mycelColor('--mycel-chart-2'),
          'chart-3': mycelColor('--mycel-chart-3'),
          'chart-4': mycelColor('--mycel-chart-4'),
          'chart-5': mycelColor('--mycel-chart-5'),
          'chart-6': mycelColor('--mycel-chart-6'),
          'chart-7': mycelColor('--mycel-chart-7'),
        },
      },
      boxShadow: {
        'mycel-sm': 'var(--mycel-shadow-sm)',
        'mycel': 'var(--mycel-shadow)',
        'mycel-lg': 'var(--mycel-shadow-lg)',
      },
    },
  },
  plugins: [],
} satisfies Config;
