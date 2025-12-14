import { Link } from 'react-router-dom';
import { motion } from 'framer-motion';
import {
  Cpu,
  Thermometer,
  Droplets,
  Wifi,
  Clock,
  Trash2,
  ExternalLink,
  HardDrive,
} from 'lucide-react';
import { useDateFormat } from '../utils/dateFormat';
import { useTranslation } from '../contexts/settingsStore';
import type { Device } from '../types';

interface DeviceCardProps {
  device: Device;
  isOnline: boolean;
  showActions?: boolean;
  onDelete?: () => void;
  temperature?: number | null;
  humidity?: number | null;
  rssi?: number | null;
  freeHeap?: number | null;
}

export default function DeviceCard({ 
  device, 
  isOnline, 
  showActions, 
  onDelete,
  temperature,
  humidity,
  rssi,
  freeHeap
}: DeviceCardProps) {
  const { formatRelative } = useDateFormat();
  const t = useTranslation();

  // Round to nearest whole number (24.1 → 24, 24.5 → 25)
  const formatTemp = (temp?: number | null) => 
    temp != null ? `${Math.round(temp)}°C` : '--°C';
  const formatHum = (hum?: number | null) => 
    hum != null ? `${hum.toFixed(0)}%` : '--%';
  const formatRssi = (r?: number | null) => 
    r != null ? `${r}dBm` : 'WiFi';
  
  // For gateway: humidity = memory %, free_heap/10 = CPU %
  const isGateway = device.device_type === 'gateway';
  const formatCpuUsage = (heap?: number | null) => 
    heap != null ? `${(heap / 10).toFixed(0)}%` : '--%';
  const formatMemUsage = (mem?: number | null) => 
    mem != null ? `${mem.toFixed(0)}%` : '--%';
  
  return (
    <motion.div
      whileHover={{ scale: 1.02 }}
      className={`glass rounded-xl p-5 card-hover relative overflow-hidden ${
        isOnline ? 'border-green-500/30' : 'border-dark-600/50'
      }`}
    >
      {/* Status indicator */}
      <div className={`absolute top-0 right-0 w-24 h-24 rounded-full blur-3xl -translate-y-1/2 translate-x-1/2 ${
        isOnline ? 'bg-green-500/20' : 'bg-red-500/10'
      }`} />

      <div className="relative">
        {/* Header */}
        <div className="flex items-start justify-between mb-4">
          <div className="flex items-center gap-3">
            <div className={`p-2.5 rounded-xl ${
              isOnline 
                ? 'bg-gradient-to-br from-green-500/20 to-emerald-500/20' 
                : 'bg-dark-700'
            }`}>
              <Cpu className={`w-5 h-5 ${isOnline ? 'text-green-400' : 'text-dark-400'}`} />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h3 className="font-semibold">{device.name}</h3>
                {device.device_type === 'gateway' && (
                  <span className="px-1.5 py-0.5 rounded text-[10px] font-medium bg-yellow-500/20 text-yellow-400 border border-yellow-500/30">
                    🌐
                  </span>
                )}
                {device.device_type === 'skud' && (
                  <span className="px-1.5 py-0.5 rounded text-[10px] font-medium bg-blue-500/20 text-blue-400 border border-blue-500/30">
                    🔐
                  </span>
                )}
              </div>
              <p className="text-xs text-dark-400">
                {device.device_type === 'gateway' 
                  ? 'Raspberry Pi' 
                  : device.device_type === 'skud'
                    ? 'Access Control'
                    : (device.platform || 'ESP')} • {device.chip_id?.slice(0, 8) || 'Unknown'}
              </p>
            </div>
          </div>
          <span className={`px-2.5 py-1 rounded-full text-xs font-medium ${
            isOnline 
              ? 'bg-green-500/20 text-green-400' 
              : 'bg-red-500/20 text-red-400'
          }`}>
            {isOnline ? 'Online' : 'Offline'}
          </span>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-3 gap-3 mb-4">
          {isGateway ? (
            <>
              {/* Gateway: CPU Temp, Memory %, CPU % */}
              <div className="text-center p-2 rounded-lg bg-dark-800/50">
                <Thermometer className="w-4 h-4 text-orange-400 mx-auto mb-1" />
                <p className="text-sm font-medium">{formatTemp(temperature)}</p>
              </div>
              <div className="text-center p-2 rounded-lg bg-dark-800/50">
                <HardDrive className="w-4 h-4 text-blue-400 mx-auto mb-1" />
                <p className="text-sm font-medium">{formatMemUsage(humidity)}</p>
              </div>
              <div className="text-center p-2 rounded-lg bg-dark-800/50">
                <Cpu className="w-4 h-4 text-green-400 mx-auto mb-1" />
                <p className="text-sm font-medium">{formatCpuUsage(freeHeap)}</p>
              </div>
            </>
          ) : (
            <>
              {/* Regular device: Temp, Humidity, WiFi */}
              <div className="text-center p-2 rounded-lg bg-dark-800/50">
                <Thermometer className="w-4 h-4 text-orange-400 mx-auto mb-1" />
                <p className="text-sm font-medium">{formatTemp(temperature)}</p>
              </div>
              <div className="text-center p-2 rounded-lg bg-dark-800/50">
                <Droplets className="w-4 h-4 text-cyan-400 mx-auto mb-1" />
                <p className="text-sm font-medium">{formatHum(humidity)}</p>
              </div>
              <div className="text-center p-2 rounded-lg bg-dark-800/50">
                <Wifi className="w-4 h-4 text-purple-400 mx-auto mb-1" />
                <p className="text-sm font-medium">{formatRssi(rssi)}</p>
              </div>
            </>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between pt-3 border-t border-dark-700/50">
          <div className="flex items-center gap-1 text-xs text-dark-400">
            <Clock className="w-3 h-3" />
            {device.last_seen 
              ? formatRelative(device.last_seen)
              : t.neverConnected
            }
          </div>
          <div className="flex items-center gap-2">
            {showActions && onDelete && (
              <button
                onClick={(e) => {
                  e.preventDefault();
                  onDelete();
                }}
                className="p-1.5 rounded-lg hover:bg-red-500/20 text-dark-400 hover:text-red-400 transition-colors"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            )}
            <Link
              to={`/devices/${device.id}`}
              className="p-1.5 rounded-lg hover:bg-primary-500/20 text-dark-400 hover:text-primary-400 transition-colors"
            >
              <ExternalLink className="w-4 h-4" />
            </Link>
          </div>
        </div>
      </div>
    </motion.div>
  );
}

