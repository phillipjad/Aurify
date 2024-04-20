/** @type {import('tailwindcss').Config} */

import colors from 'tailwindcss/colors'

export default {
  content: [
    "./src/**/*.{html,js,jsx,tsx}",
    "./index.html"
  ],
  theme: {
    colors,
    extend: {},
  },
  plugins: [],
}

