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
      },
      animation: {
        'slide-in': 'slide-in 0.2s ease-out',
        'fade-in': 'fade-in 0.25s ease-out',
        'expand-height': 'expand-height 0.3s ease-out',
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
          warning: 'var(--mycel-warning)',
          error: 'var(--mycel-error)',
          live: 'var(--mycel-live)',
          info: 'var(--mycel-info)',
          overlay: 'var(--mycel-overlay)',
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
