import React from 'react'
import ReactDOM from 'react-dom/client'
import { CssBaseline, ThemeProvider, createTheme } from '@mui/material'
import App from './App'

const theme = createTheme({
  palette: {
    mode: 'dark',
    primary: {
      main: '#FF9900', // AWS Orange
    },
    secondary: {
      main: '#232F3E', // AWS Dark Blue
    },
    background: {
      default: '#1a1a2e',
      paper: '#232F3E',
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <App />
    </ThemeProvider>
  </React.StrictMode>,
)
