/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './src/pages/**/*.{js,ts,jsx,tsx,mdx}',
    './src/components/**/*.{js,ts,jsx,tsx,mdx}',
    './src/app/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      colors: {
        // SIN-Code design tokens (mirrored from web/theme/tokens.css and internal/tui/theme/tokens.go)
        sin: {
          base: 'var(--sin-color-base)',
          surface: 'var(--sin-color-surface)',
          text: 'var(--sin-color-text)',
          textMuted: 'var(--sin-color-text-muted)',
          primary: 'var(--sin-color-primary)',
          primaryHover: 'var(--sin-color-primary-hover)',
          accent: 'var(--sin-color-accent)',
          accentHover: 'var(--sin-color-accent-hover)',
          success: 'var(--sin-color-success)',
          warning: 'var(--sin-color-warning)',
          danger: 'var(--sin-color-danger)',
          info: 'var(--sin-color-info)',
          border: 'var(--sin-color-border)',
          focus: 'var(--sin-color-focus)',
        },
      },
      spacing: {
        'sin-0': 'var(--sin-space-0)',
        'sin-1': 'var(--sin-space-1)',
        'sin-2': 'var(--sin-space-2)',
        'sin-3': 'var(--sin-space-3)',
        'sin-4': 'var(--sin-space-4)',
        'sin-5': 'var(--sin-space-5)',
        'sin-6': 'var(--sin-space-6)',
        'sin-7': 'var(--sin-space-7)',
        'sin-8': 'var(--sin-space-8)',
      },
      borderRadius: {
        'sin-sm': 'var(--sin-radius-sm)',
        'sin-md': 'var(--sin-radius-md)',
        'sin-lg': 'var(--sin-radius-lg)',
        'sin-xl': 'var(--sin-radius-xl)',
      },
      fontFamily: {
        sans: ['var(--sin-font-sans)', 'system-ui', 'sans-serif'],
        mono: ['var(--sin-font-mono)', 'monospace'],
      },
      fontSize: {
        'sin-xs': ['var(--sin-text-xs)', { lineHeight: '1.5' }],
        'sin-sm': ['var(--sin-text-sm)', { lineHeight: '1.5' }],
        'sin-base': ['var(--sin-text-base)', { lineHeight: '1.5' }],
        'sin-lg': ['var(--sin-text-lg)', { lineHeight: '1.5' }],
        'sin-xl': ['var(--sin-text-xl)', { lineHeight: '1.4' }],
        'sin-2xl': ['var(--sin-text-2xl)', { lineHeight: '1.3' }],
        'sin-3xl': ['var(--sin-text-3xl)', { lineHeight: '1.2' }],
      },
      transitionDuration: {
        'sin-fast': 'var(--sin-duration-fast)',
        'sin-base': 'var(--sin-duration-base)',
        'sin-slow': 'var(--sin-duration-slow)',
      },
      transitionTimingFunction: {
        'sin-in': 'var(--sin-ease-in)',
        'sin-out': 'var(--sin-ease-out)',
        'sin-in-out': 'var(--sin-ease-in-out)',
      },
    },
  },
  plugins: [],
};