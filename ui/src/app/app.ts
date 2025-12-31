import { Component, OnInit, OnDestroy, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import {
  DockerExtensionService,
  Profile,
  Status,
  Credentials,
  EnvironmentInfo,
  Settings,
  CredentialSource,
} from './services/docker-extension.service';

@Component({
  selector: 'app-root',
  imports: [CommonModule, FormsModule],
  templateUrl: './app.html',
  styleUrl: './app.scss',
})
export class App implements OnInit, OnDestroy {
  profiles = signal<Profile[]>([]);
  statuses = signal<Status[]>([]);
  selectedProfile = signal('default');
  tokenCode = signal('');
  loading = signal(false);
  error = signal<string | null>(null);
  success = signal<string | null>(null);
  credentials = signal<Credentials | null>(null);

  // Environment and settings
  environment = signal<EnvironmentInfo | null>(null);
  settings = signal<Settings | null>(null);
  showSettings = signal(false);
  settingsLoading = signal(false);

  private refreshInterval: ReturnType<typeof setInterval> | null = null;

  constructor(private dockerService: DockerExtensionService) {}

  ngOnInit(): void {
    this.fetchEnvironment();
    this.fetchSettings();
    this.refreshAll();
    this.refreshInterval = setInterval(() => this.fetchStatuses(), 30000);
  }

  ngOnDestroy(): void {
    if (this.refreshInterval) {
      clearInterval(this.refreshInterval);
    }
  }

  async fetchEnvironment(): Promise<void> {
    try {
      const env = await this.dockerService.getEnvironment();
      this.environment.set(env);
    } catch (err) {
      console.error('Failed to fetch environment:', err);
    }
  }

  async fetchSettings(): Promise<void> {
    try {
      const settings = await this.dockerService.getSettings();
      this.settings.set(settings);
    } catch (err) {
      console.error('Failed to fetch settings:', err);
    }
  }

  async refreshAll(): Promise<void> {
    await Promise.all([this.fetchProfiles(), this.fetchStatuses()]);
  }

  async fetchProfiles(): Promise<void> {
    try {
      const profiles = await this.dockerService.getProfiles();
      this.profiles.set(profiles);
      if (profiles.length > 0 && !profiles.find((p) => p.name === this.selectedProfile())) {
        this.selectedProfile.set(profiles[0].name);
      }
    } catch (err) {
      console.error('Failed to fetch profiles:', err);
    }
  }

  async fetchStatuses(): Promise<void> {
    try {
      const statuses = await this.dockerService.getAllStatuses();
      this.statuses.set(statuses);
    } catch (err) {
      console.error('Failed to fetch statuses:', err);
    }
  }

  getStatusForProfile(profileName: string): Status | undefined {
    return this.statuses().find((s) => s.profile === profileName);
  }

  getSelectedProfileInfo(): Profile | undefined {
    return this.profiles().find((p) => p.name === this.selectedProfile());
  }

  async handleLogin(): Promise<void> {
    if (!this.tokenCode()) {
      this.error.set('Please enter your MFA token code');
      return;
    }

    this.loading.set(true);
    this.error.set(null);
    this.success.set(null);

    try {
      await this.dockerService.login({
        profile: this.selectedProfile(),
        tokenCode: this.tokenCode(),
      });
      this.success.set(`Successfully authenticated profile: ${this.selectedProfile()}`);
      this.tokenCode.set('');
      await this.fetchStatuses();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Authentication failed';
      this.error.set(errorMessage);
    } finally {
      this.loading.set(false);
    }
  }

  async handleClearCredentials(profile: string): Promise<void> {
    try {
      await this.dockerService.clearCredentials(profile);
      this.success.set(`Cleared credentials for: ${profile}`);
      await this.fetchStatuses();
      if (this.credentials()?.profile === profile) {
        this.credentials.set(null);
      }
    } catch (err) {
      this.error.set('Failed to clear credentials');
    }
  }

  async handleViewCredentials(profile: string): Promise<void> {
    try {
      const creds = await this.dockerService.getCredentials(profile);
      this.credentials.set(creds);
    } catch (err) {
      this.error.set('Failed to fetch credentials');
    }
  }

  async handleCopyEnv(): Promise<void> {
    const creds = this.credentials();
    if (!creds) return;

    const envString = `AWS_ACCESS_KEY_ID=${creds.accessKeyId}\nAWS_SECRET_ACCESS_KEY=${creds.secretAccessKey}\nAWS_SESSION_TOKEN=${creds.sessionToken}`;
    await this.dockerService.copyToClipboard(envString);
    this.success.set('Credentials copied to clipboard!');
  }

  async handleExportEnvFile(profile: string): Promise<void> {
    try {
      await this.dockerService.exportEnvFile(profile, './aws.env');
      this.success.set('Exported to ./aws.env in current directory');
    } catch (err) {
      this.error.set('Failed to export env file');
    }
  }

  closeCredentials(): void {
    this.credentials.set(null);
  }

  clearError(): void {
    this.error.set(null);
  }

  clearSuccess(): void {
    this.success.set(null);
  }

  onTokenKeyPress(event: KeyboardEvent): void {
    if (event.key === 'Enter') {
      this.handleLogin();
    }
  }

  // Settings management

  toggleSettings(): void {
    this.showSettings.set(!this.showSettings());
  }

  async updateCredentialSource(source: CredentialSource): Promise<void> {
    const currentSettings = this.settings();
    if (!currentSettings) return;

    this.settingsLoading.set(true);
    try {
      const newSettings = await this.dockerService.updateSettings({
        ...currentSettings,
        credentialSource: source,
      });
      this.settings.set(newSettings);
      this.success.set(`Credential source updated to: ${source}`);
      // Refresh profiles with new source
      await this.fetchEnvironment();
      await this.fetchProfiles();
    } catch (err) {
      this.error.set('Failed to update settings');
    } finally {
      this.settingsLoading.set(false);
    }
  }

  async updateCustomPaths(configPath: string, credsPath: string): Promise<void> {
    const currentSettings = this.settings();
    if (!currentSettings) return;

    this.settingsLoading.set(true);
    try {
      const newSettings = await this.dockerService.updateSettings({
        ...currentSettings,
        credentialSource: 'custom',
        customConfigPath: configPath,
        customCredsPath: credsPath,
      });
      this.settings.set(newSettings);
      this.success.set('Custom paths updated');
      await this.fetchProfiles();
    } catch (err) {
      this.error.set('Failed to update custom paths');
    } finally {
      this.settingsLoading.set(false);
    }
  }

  getEnvironmentLabel(): string {
    const env = this.environment();
    if (!env) return 'Loading...';
    if (env.isWsl2) return 'WSL2';
    if (env.isWindows) return 'Windows';
    if (env.isMacOS) return 'macOS';
    if (env.isLinux) return 'Linux';
    return 'Unknown';
  }

  getSourceLabel(source: CredentialSource): string {
    switch (source) {
      case 'auto':
        return 'Auto-detect';
      case 'linux':
        return 'Linux (~/.aws)';
      case 'wsl2':
        return 'WSL2 Linux';
      case 'windows':
        return 'Windows';
      case 'custom':
        return 'Custom Path';
      default:
        return source;
    }
  }
}
