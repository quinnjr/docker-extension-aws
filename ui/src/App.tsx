import { useState, useEffect, useCallback } from 'react'
import { createDockerDesktopClient } from '@docker/extension-api-client'
import {
  Box,
  Card,
  CardContent,
  Typography,
  TextField,
  Button,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Alert,
  Chip,
  IconButton,
  Tooltip,
  CircularProgress,
  Stack,
  Divider,
  Paper,
} from '@mui/material'
import {
  Refresh as RefreshIcon,
  ContentCopy as CopyIcon,
  Delete as DeleteIcon,
  CheckCircle as CheckCircleIcon,
  Error as ErrorIcon,
  Schedule as ScheduleIcon,
  VpnKey as KeyIcon,
} from '@mui/icons-material'

interface Profile {
  name: string
  region: string
  mfaSerial: string
}

interface Status {
  profile: string
  authenticated: boolean
  expiration?: string
  timeRemaining?: string
}

interface Credentials {
  accessKeyId: string
  secretAccessKey: string
  sessionToken: string
  expiration: string
}

const ddClient = createDockerDesktopClient()

function App() {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [statuses, setStatuses] = useState<Status[]>([])
  const [selectedProfile, setSelectedProfile] = useState('default')
  const [tokenCode, setTokenCode] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [credentials, setCredentials] = useState<Credentials | null>(null)

  const fetchProfiles = useCallback(async () => {
    try {
      const response = await ddClient.extension.vm?.service?.get('/profiles')
      setProfiles(response as Profile[])
      if (response && (response as Profile[]).length > 0) {
        setSelectedProfile((response as Profile[])[0].name)
      }
    } catch (err) {
      console.error('Failed to fetch profiles:', err)
    }
  }, [])

  const fetchStatuses = useCallback(async () => {
    try {
      const response = await ddClient.extension.vm?.service?.get('/status/all')
      setStatuses(response as Status[])
    } catch (err) {
      console.error('Failed to fetch statuses:', err)
    }
  }, [])

  const refreshAll = useCallback(() => {
    fetchProfiles()
    fetchStatuses()
  }, [fetchProfiles, fetchStatuses])

  useEffect(() => {
    refreshAll()
    const interval = setInterval(fetchStatuses, 30000) // Refresh every 30s
    return () => clearInterval(interval)
  }, [refreshAll, fetchStatuses])

  const handleLogin = async () => {
    if (!tokenCode) {
      setError('Please enter your MFA token code')
      return
    }

    setLoading(true)
    setError(null)
    setSuccess(null)

    try {
      await ddClient.extension.vm?.service?.post('/login', {
        profile: selectedProfile,
        tokenCode: tokenCode,
      })
      setSuccess(`Successfully authenticated profile: ${selectedProfile}`)
      setTokenCode('')
      fetchStatuses()
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : 'Authentication failed'
      setError(errorMessage)
    } finally {
      setLoading(false)
    }
  }

  const handleClearCredentials = async (profile: string) => {
    try {
      await ddClient.extension.vm?.service?.delete(`/credentials?profile=${profile}`)
      setSuccess(`Cleared credentials for: ${profile}`)
      fetchStatuses()
      if (credentials?.profile === profile) {
        setCredentials(null)
      }
    } catch (err) {
      setError('Failed to clear credentials')
    }
  }

  const handleViewCredentials = async (profile: string) => {
    try {
      const response = await ddClient.extension.vm?.service?.get(`/credentials?profile=${profile}`)
      setCredentials({ ...(response as Credentials), profile } as Credentials & { profile: string })
    } catch (err) {
      setError('Failed to fetch credentials')
    }
  }

  const handleCopyEnv = async () => {
    if (!credentials) return
    const envString = `AWS_ACCESS_KEY_ID=${credentials.accessKeyId}\nAWS_SECRET_ACCESS_KEY=${credentials.secretAccessKey}\nAWS_SESSION_TOKEN=${credentials.sessionToken}`
    await navigator.clipboard.writeText(envString)
    setSuccess('Credentials copied to clipboard!')
  }

  const handleExportEnvFile = async (profile: string) => {
    try {
      const result = await ddClient.extension.host?.cli.exec('docker-aws', ['env', '-p', profile, '-o', './aws.env'])
      if (result?.stderr) {
        setError(result.stderr)
      } else {
        setSuccess('Exported to ./aws.env in current directory')
      }
    } catch (err) {
      setError('Failed to export env file')
    }
  }

  const getStatusForProfile = (profileName: string) => {
    return statuses.find(s => s.profile === profileName)
  }

  return (
    <Box sx={{ p: 3, maxWidth: 1200, margin: '0 auto' }}>
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 3 }}>
        <KeyIcon sx={{ fontSize: 40, color: 'primary.main', mr: 2 }} />
        <Typography variant="h4" component="h1">
          AWS MFA Credentials
        </Typography>
        <Box sx={{ flexGrow: 1 }} />
        <Tooltip title="Refresh">
          <IconButton onClick={refreshAll} color="primary">
            <RefreshIcon />
          </IconButton>
        </Tooltip>
      </Box>

      {error && (
        <Alert severity="error" onClose={() => setError(null)} sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {success && (
        <Alert severity="success" onClose={() => setSuccess(null)} sx={{ mb: 2 }}>
          {success}
        </Alert>
      )}

      <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
        {/* Login Card */}
        <Card sx={{ flex: 1, minWidth: 300 }}>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              Authenticate with MFA
            </Typography>

            <FormControl fullWidth sx={{ mb: 2 }}>
              <InputLabel>AWS Profile</InputLabel>
              <Select
                value={selectedProfile}
                label="AWS Profile"
                onChange={(e) => setSelectedProfile(e.target.value)}
              >
                {profiles.map((profile) => (
                  <MenuItem key={profile.name} value={profile.name}>
                    {profile.name} ({profile.region})
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <TextField
              fullWidth
              label="MFA Token Code"
              value={tokenCode}
              onChange={(e) => setTokenCode(e.target.value)}
              placeholder="Enter 6-digit code"
              inputProps={{ maxLength: 6 }}
              sx={{ mb: 2 }}
              onKeyPress={(e) => {
                if (e.key === 'Enter') handleLogin()
              }}
            />

            <Button
              variant="contained"
              fullWidth
              onClick={handleLogin}
              disabled={loading || !tokenCode}
              startIcon={loading ? <CircularProgress size={20} /> : <KeyIcon />}
            >
              {loading ? 'Authenticating...' : 'Login with MFA'}
            </Button>

            {selectedProfile && profiles.find(p => p.name === selectedProfile) && (
              <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                MFA: {profiles.find(p => p.name === selectedProfile)?.mfaSerial}
              </Typography>
            )}
          </CardContent>
        </Card>

        {/* Profiles Status Card */}
        <Card sx={{ flex: 2, minWidth: 400 }}>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              Profile Status
            </Typography>

            <Stack spacing={2}>
              {profiles.map((profile) => {
                const status = getStatusForProfile(profile.name)
                return (
                  <Paper
                    key={profile.name}
                    sx={{
                      p: 2,
                      bgcolor: status?.authenticated ? 'rgba(46, 125, 50, 0.1)' : 'rgba(211, 47, 47, 0.1)',
                      border: 1,
                      borderColor: status?.authenticated ? 'success.main' : 'error.main',
                    }}
                  >
                    <Box sx={{ display: 'flex', alignItems: 'center' }}>
                      {status?.authenticated ? (
                        <CheckCircleIcon color="success" sx={{ mr: 1 }} />
                      ) : (
                        <ErrorIcon color="error" sx={{ mr: 1 }} />
                      )}
                      <Typography variant="subtitle1" sx={{ fontWeight: 'bold' }}>
                        {profile.name}
                      </Typography>
                      <Chip
                        size="small"
                        label={profile.region}
                        sx={{ ml: 1 }}
                      />
                      <Box sx={{ flexGrow: 1 }} />
                      {status?.authenticated && (
                        <>
                          <Tooltip title="View credentials">
                            <IconButton
                              size="small"
                              onClick={() => handleViewCredentials(profile.name)}
                            >
                              <KeyIcon />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Export to file">
                            <IconButton
                              size="small"
                              onClick={() => handleExportEnvFile(profile.name)}
                            >
                              <CopyIcon />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Clear credentials">
                            <IconButton
                              size="small"
                              color="error"
                              onClick={() => handleClearCredentials(profile.name)}
                            >
                              <DeleteIcon />
                            </IconButton>
                          </Tooltip>
                        </>
                      )}
                    </Box>
                    {status?.authenticated && status.timeRemaining && (
                      <Box sx={{ display: 'flex', alignItems: 'center', mt: 1 }}>
                        <ScheduleIcon fontSize="small" sx={{ mr: 0.5, color: 'text.secondary' }} />
                        <Typography variant="body2" color="text.secondary">
                          Expires in: {status.timeRemaining}
                        </Typography>
                      </Box>
                    )}
                  </Paper>
                )
              })}

              {profiles.length === 0 && (
                <Typography color="text.secondary" align="center">
                  No AWS profiles with MFA configured found.
                  <br />
                  Add mfa_serial to your ~/.aws/config
                </Typography>
              )}
            </Stack>
          </CardContent>
        </Card>
      </Stack>

      {/* Credentials Display */}
      {credentials && (
        <Card sx={{ mt: 3 }}>
          <CardContent>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
              <Typography variant="h6">
                Credentials: {(credentials as Credentials & { profile?: string }).profile}
              </Typography>
              <Box sx={{ flexGrow: 1 }} />
              <Button
                startIcon={<CopyIcon />}
                onClick={handleCopyEnv}
                variant="outlined"
                size="small"
              >
                Copy as ENV
              </Button>
              <IconButton onClick={() => setCredentials(null)} sx={{ ml: 1 }}>
                <DeleteIcon />
              </IconButton>
            </Box>
            <Divider sx={{ mb: 2 }} />
            <Stack spacing={1}>
              <Box>
                <Typography variant="caption" color="text.secondary">
                  AWS_ACCESS_KEY_ID
                </Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                  {credentials.accessKeyId}
                </Typography>
              </Box>
              <Box>
                <Typography variant="caption" color="text.secondary">
                  AWS_SECRET_ACCESS_KEY
                </Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                  {credentials.secretAccessKey.substring(0, 10)}...
                </Typography>
              </Box>
              <Box>
                <Typography variant="caption" color="text.secondary">
                  AWS_SESSION_TOKEN
                </Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                  {credentials.sessionToken.substring(0, 30)}...
                </Typography>
              </Box>
            </Stack>
          </CardContent>
        </Card>
      )}

      {/* CLI Usage */}
      <Card sx={{ mt: 3 }}>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            CLI Usage
          </Typography>
          <Typography variant="body2" color="text.secondary" gutterBottom>
            This extension also installs a CLI tool. Use it from your terminal:
          </Typography>
          <Paper sx={{ p: 2, bgcolor: 'background.default', fontFamily: 'monospace', fontSize: '0.875rem' }}>
            <Box>$ docker aws login -p {selectedProfile}</Box>
            <Box>$ docker aws run -- -it amazon/aws-cli s3 ls</Box>
            <Box>$ docker aws compose -- up -d</Box>
            <Box>$ eval $(docker aws env --export)</Box>
          </Paper>
        </CardContent>
      </Card>
    </Box>
  )
}

export default App
