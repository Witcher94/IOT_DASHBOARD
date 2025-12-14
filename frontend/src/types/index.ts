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
  device_type?: string; // simple_device, gateway, mesh_node, skud
  gateway_id?: string; // For mesh nodes
  mesh_node_id?: number; // painlessMesh node ID
  chip_id?: string;
  mac?: string;
  platform?: string;
  firmware?: string;
  is_online: boolean;
  last_seen?: string;
  dht_enabled: boolean;
  mesh_enabled: boolean;
  // Alert settings
  alerts_enabled: boolean;
  alert_temp_min?: number;
  alert_temp_max?: number;
  alert_humidity_max?: number;
  created_at: string;
  updated_at: string;
}

export interface DeviceShare {
  id: string;
  device_id: string;
  owner_id: string;
  shared_with_id: string;
  permission: 'view' | 'edit';
  created_at: string;
  shared_with_name?: string;
  shared_with_email?: string;
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
  device_type?: string; // simple_device, gateway, skud
}

export type DeviceType = 'simple_device' | 'gateway' | 'mesh_node' | 'skud';

export interface GatewayTopology {
  gateway: Device;
  mesh_nodes: Device[];
  total_nodes: number;
  online_nodes: number;
}

export interface CreateCommandRequest {
  command: string;
  firmware_url?: string;
  interval?: number;
  name?: string;
}

export interface WebSocketMessage {
  type: 'metrics' | 'device_status' | 'access_log';
  device_id?: string;
  data: Record<string, unknown>;
}

// ==================== SKUD (Access Control) Types ====================

export type CardStatus = 'pending' | 'active' | 'disabled';

export interface Card {
  id: string;
  card_uid: string;
  card_type: string; // MIFARE_CLASSIC_1K, MIFARE_DESFIRE, etc.
  name: string;
  status: CardStatus;
  devices?: DeviceBrief[];
  created_at: string;
  updated_at: string;
}

export interface UpdateCardRequest {
  name?: string;
  status?: CardStatus;
}

export interface DeviceBrief {
  id: string;
  device_id: string;
  name: string;
}

export interface AccessDevice {
  id: string;
  device_id: string;
  secret_key: string;
  name?: string;
  created_at: string;
}

export interface AccessLog {
  id: string;
  device_id: string;
  card_uid: string;
  card_type: string;
  action: string;
  status: string;
  allowed: boolean;
  created_at: string;
}

export interface CreateAccessDeviceRequest {
  device_id: string;
  secret_key: string;
  name?: string;
}

export interface UpdateCardStatusRequest {
  status: CardStatus;
}

