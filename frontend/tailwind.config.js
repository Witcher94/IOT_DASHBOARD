/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#eef9ff',
          100: '#d8f1ff',
          200: '#b9e7ff',
          300: '#89d9ff',
          400: '#52c2ff',
          500: '#2aa4ff',
          600: '#1485f7',
          700: '#0d6de3',
          800: '#1258b8',
          900: '#154b91',
          950: '#122e58',
        },
        accent: {
          400: '#63ffa3',
          500: '#22c55e',
          600: '#16a34a',
        },
        dark: {
          50: '#f6f6f7',
          100: '#e1e3e6',
          200: '#c3c6cd',
          300: '#9ea2ad',
          400: '#797e8c',
          500: '#5f6371',
          600: '#4b4e5a',
          700: '#3e404a',
          800: '#35363e',
          900: '#1a1a2e',
          950: '#0a0a12',
        },
      },
      fontFamily: {
        sans: ['DM Sans', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
      animation: {
        'glow': 'glow 2s ease-in-out infinite alternate',
        'float': 'float 6s ease-in-out infinite',
        'pulse-slow': 'pulse 4s cubic-bezier(0.4, 0, 0.6, 1) infinite',
      },
      keyframes: {
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(78, 195, 255, 0.3)' },
          '100%': { boxShadow: '0 0 40px rgba(78, 195, 255, 0.6)' },
        },
        float: {
          '0%, 100%': { transform: 'translateY(0px)' },
          '50%': { transform: 'translateY(-10px)' },
        },
      },
    },
  },
  plugins: [],
}



