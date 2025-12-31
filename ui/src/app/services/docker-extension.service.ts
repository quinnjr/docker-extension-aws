import { Injectable } from '@angular/core';
import { createDockerDesktopClient } from '@docker/extension-api-client';

export type CredentialSource = 'auto' | 'linux' | 'wsl2' | 'windows' | 'custom';

export interface AWSPathInfo {
  source: CredentialSource;
  configPath: string;
  credsPath: string;
  exists: boolean;
  description: string;
}

export interface EnvironmentInfo {
  isWsl2: boolean;
  isWindows: boolean;
  isLinux: boolean;
  isMacOS: boolean;
  wsl2Distros?: string[];
  detectedPaths: AWSPathInfo[];
  activeSource: CredentialSource;
  homeDir: string;
  windowsHomeDir?: string;
}

export interface Settings {
  credentialSource: CredentialSource;
  customConfigPath?: string;
  customCredsPath?: string;
  wsl2Distro?: string;
}

export interface Profile {
  name: string;
  region: string;
  mfaSerial: string;
  source?: string;
}

export interface Status {
  profile: string;
  authenticated: boolean;
  expiration?: string;
  timeRemaining?: string;
}

export interface Credentials {
  accessKeyId: string;
  secretAccessKey: string;
  sessionToken: string;
  expiration: string;
  profile?: string;
}

export interface LoginRequest {
  profile: string;
  tokenCode: string;
  duration?: number;
}

@Injectable({
  providedIn: 'root',
})
export class DockerExtensionService {
  private ddClient = createDockerDesktopClient();

  // Environment and Settings

  async getEnvironment(): Promise<EnvironmentInfo> {
    const response = await this.ddClient.extension.vm?.service?.get('/environment');
    return response as EnvironmentInfo;
  }

  async getSettings(): Promise<Settings> {
    const response = await this.ddClient.extension.vm?.service?.get('/settings');
    return response as Settings;
  }

  async updateSettings(settings: Settings): Promise<Settings> {
    const response = await this.ddClient.extension.vm?.service?.put('/settings', settings);
    return response as Settings;
  }

  // Profiles and Authentication

  async getProfiles(): Promise<Profile[]> {
    const response = await this.ddClient.extension.vm?.service?.get('/profiles');
    return response as Profile[];
  }

  async getStatus(profile: string): Promise<Status> {
    const response = await this.ddClient.extension.vm?.service?.get(
      `/status?profile=${profile}`
    );
    return response as Status;
  }

  async getAllStatuses(): Promise<Status[]> {
    const response = await this.ddClient.extension.vm?.service?.get('/status/all');
    return response as Status[];
  }

  async login(request: LoginRequest): Promise<Status> {
    const response = await this.ddClient.extension.vm?.service?.post('/login', request);
    return response as Status;
  }

  async getCredentials(profile: string): Promise<Credentials> {
    const response = await this.ddClient.extension.vm?.service?.get(
      `/credentials?profile=${profile}`
    );
    return { ...(response as Credentials), profile };
  }

  async clearCredentials(profile?: string): Promise<void> {
    const query = profile ? `?profile=${profile}` : '';
    await this.ddClient.extension.vm?.service?.delete(`/credentials${query}`);
  }

  async exportEnvFile(profile: string, path: string): Promise<void> {
    await this.ddClient.extension.host?.cli.exec('docker-aws', ['env', '-p', profile, '-o', path]);
  }

  async copyToClipboard(text: string): Promise<void> {
    await navigator.clipboard.writeText(text);
  }
}
