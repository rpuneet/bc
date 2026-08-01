import type { Config } from 'tailwindcss';

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
          bg: 'var(--mycel-bg)',
          surface: 'var(--mycel-surface)',
          'surface-2': 'var(--mycel-surface-2)',
          'surface-hover': 'var(--mycel-surface-hover)',
          border: 'var(--mycel-border)',
          'border-strong': 'var(--mycel-border-strong)',
          text: 'var(--mycel-text)',
          'text-2': 'var(--mycel-text-2)',
          muted: 'var(--mycel-muted)',
          accent: 'var(--mycel-accent)',
          'accent-hover': 'var(--mycel-accent-hover)',
          'accent-subtle': 'var(--mycel-accent-subtle)',
          'accent-fg': 'var(--mycel-accent-fg)',
          success: 'var(--mycel-success)',
          'success-subtle': 'var(--mycel-success-subtle)',
          warning: 'var(--mycel-warning)',
          'warning-subtle': 'var(--mycel-warning-subtle)',
          error: 'var(--mycel-error)',
          'error-subtle': 'var(--mycel-error-subtle)',
          live: 'var(--mycel-live)',
          info: 'var(--mycel-info)',
          'info-subtle': 'var(--mycel-info-subtle)',
          overlay: 'var(--mycel-overlay)',
          'chart-1': 'var(--mycel-chart-1)',
          'chart-2': 'var(--mycel-chart-2)',
          'chart-3': 'var(--mycel-chart-3)',
          'chart-4': 'var(--mycel-chart-4)',
          'chart-5': 'var(--mycel-chart-5)',
          'chart-6': 'var(--mycel-chart-6)',
          'chart-7': 'var(--mycel-chart-7)',
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
