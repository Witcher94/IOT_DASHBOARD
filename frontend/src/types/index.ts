export interface User {
  id: string;
  email: string;
  name: string;
  picture?: string;
  is_admin: boolean;
  created_at: string;
  updated_at: string;
}

export interface Device {
  id: string;
  user_id: string;
  name: string;
  token: string;
  chip_id?: string;
  mac?: string;
  platform?: string;
  firmware?: string;
  is_online: boolean;
  last_seen?: string;
  dht_enabled: boolean;
  mesh_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface Metric {
  id: string;
  device_id: string;
  temperature?: number;
  humidity?: number;
  rssi?: number;
  free_heap?: number;
  wifi_scan?: WifiNetwork[];
  mesh_neighbors?: MeshNeighbor[];
  created_at: string;
}

export interface WifiNetwork {
  ssid: string;
  rssi: number;
  bssid: string;
  channel: number;
  enc: string;
}

export interface MeshNeighbor {
  id: number;
}

export interface Command {
  id: string;
  device_id: string;
  command: string;
  params?: string;
  status: string;
  created_at: string;
  sent_at?: string;
  acked_at?: string;
}

export interface DashboardStats {
  total_devices: number;
  online_devices: number;
  total_users: number;
  avg_temperature: number;
  avg_humidity: number;
}

export interface CreateDeviceRequest {
  name: string;
}

export interface CreateCommandRequest {
  command: string;
  firmware_url?: string;
  interval?: number;
  name?: string;
}

export interface WebSocketMessage {
  type: 'metrics' | 'device_status';
  device_id?: string;
  data: Record<string, unknown>;
}

