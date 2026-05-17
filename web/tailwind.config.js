/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        display: ['"Cabinet Grotesk"', 'system-ui', 'sans-serif'],
        body: ['Geist', '"PingFang SC"', '"Microsoft YaHei"', 'sans-serif'],
        mono: ['"JetBrains Mono"', '"Fira Code"', 'monospace'],
      },
      colors: {
        // ── New Warm Industrial palette ──
        brand: {
          DEFAULT: '#c9954a',
          50:  '#fdf8f0',
          100: '#f9edda',
          200: '#f2d8b0',
          300: '#e8bc7d',
          400: '#dca45c',
          500: '#c9954a',
          600: '#b07a38',
          700: '#936031',
          800: '#794e2d',
          900: '#654128',
          950: '#3a2214',
        },
        sage: {
          DEFAULT: '#7a9a8a',
          50:  '#f3f7f4',
          100: '#e4ece6',
          200: '#c9d9ce',
          300: '#a6bfad',
          400: '#7a9a8a',
          500: '#5e7e6e',
          600: '#496558',
          700: '#3c5247',
          800: '#334339',
          900: '#2c3831',
          950: '#151f19',
        },
        surface: {
          DEFAULT: '#1a1a18',
          50:  '#2e2e2a',
          100: '#2a2a26',
          200: '#242420',
          300: '#1e1e1b',
          400: '#1a1a18',
          500: '#161614',
          600: '#111110',
          // Backward-compat (old 700-950 → warm charcoal)
          700: '#242420',
          800: '#1a1a18',
          900: '#111110',
          950: '#0d0d0c',
        },
        cream: {
          DEFAULT: '#e8e4d9',
          50:  '#faf8f4',
          100: '#f3f0e8',
          200: '#e8e4d9',
          300: '#c4bfb5',
          400: '#a09b92',
          500: '#7d7871',
          600: '#605c56',
          700: '#4a4742',
          800: '#363330',
          900: '#242220',
        },

        // ── Backward-compat aliases (old class names → new warm colors) ──
        primary: {
          400: '#dca45c',  // was cyan → now brand-400
          500: '#c9954a',  // was cyan → now brand-500
          600: '#b07a38',  // was cyan → now brand-600
        },
        accent: {
          400: '#7a9a8a',  // was purple → now sage-400
          500: '#5e7e6e',  // was purple → now sage-500
        },
      },
      fontSize: {
        '2xs': ['0.625rem', { lineHeight: '0.875rem' }],
      },
      spacing: {
        '18': '4.5rem',
        '88': '22rem',
      },
      transitionDuration: {
        '400': '400ms',
      },
      animation: {
        'slide-in-left': 'slideInLeft 0.3s ease-out',
        'fade-in': 'fadeIn 0.4s ease-out',
        'scale-in': 'scaleIn 0.3s ease-out',
      },
      keyframes: {
        slideInLeft: {
          '0%': { opacity: '0', transform: 'translateX(-8px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' },
        },
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.96)' },
          '100%': { opacity: '1', transform: 'scale(1)' },
        },
      },
    },
  },
  plugins: [],
}
