import { useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { motion } from 'framer-motion';
import {
  ArrowLeft,
  Cpu,
  Thermometer,
  Droplets,
  Wifi,
  RefreshCw,
  Power,
  Settings,
  Trash2,
  Send,
  Copy,
  Check,
  Bell,
  BellOff,
  X,
} from 'lucide-react';
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler,
} from 'chart.js';
import { Line } from 'react-chartjs-2';
import { format } from 'date-fns';
import toast from 'react-hot-toast';
import { devicesApi, metricsApi, commandsApi } from '../services/api';
import { useWebSocket } from '../hooks/useWebSocket';
import { useTranslation } from '../contexts/settingsStore';
import type { WebSocketMessage, Metric } from '../types';

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
);

export default function DeviceDetail() {
  const t = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [selectedPeriod, setSelectedPeriod] = useState('1h');
  const [copiedToken, setCopiedToken] = useState(false);
  const [visibleToken, setVisibleToken] = useState<string | null>(null);

  const { data: device, isLoading: deviceLoading } = useQuery({
    queryKey: ['device', id],
    queryFn: () => devicesApi.getById(id!),
    enabled: !!id,
  });

  const { data: metrics, refetch: refetchMetrics } = useQuery({
    queryKey: ['metrics', id, selectedPeriod],
    queryFn: () => metricsApi.getByPeriod(id!, selectedPeriod),
    enabled: !!id,
    refetchInterval: 30000,
  });

  const { data: commands } = useQuery({
    queryKey: ['commands', id],
    queryFn: () => commandsApi.getByDeviceId(id!),
    enabled: !!id,
  });

  const sendCommandMutation = useMutation({
    mutationFn: (command: string) => commandsApi.create(id!, { command }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['commands', id] });
      toast.success('Command sent!');
    },
    onError: () => {
      toast.error('Failed to send command');
    },
  });

  const cancelCommandMutation = useMutation({
    mutationFn: (commandId: string) => commandsApi.cancel(id!, commandId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['commands', id] });
      toast.success('Command cancelled');
    },
    onError: () => {
      toast.error('Failed to cancel command');
    },
  });

  const regenerateTokenMutation = useMutation({
    mutationFn: () => devicesApi.regenerateToken(id!),
    onSuccess: (data) => {
      setVisibleToken(data.token);
      queryClient.invalidateQueries({ queryKey: ['device', id] });
      toast.success(t.tokenRegenerated);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => devicesApi.delete(id!),
    onSuccess: () => {
      toast.success(t.delete);
      navigate('/devices');
    },
  });

  const updateAlertsMutation = useMutation({
    mutationFn: (settings: {
      alerts_enabled?: boolean;
      alert_temp_min?: number;
      alert_temp_max?: number;
      alert_humidity_max?: number;
    }) => devicesApi.updateAlertSettings(id!, settings),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['device', id] });
      toast.success('Alert settings saved!');
    },
  });

  const handleWebSocketMessage = useCallback((message: WebSocketMessage) => {
    if (message.type === 'metrics' && message.device_id === id) {
      refetchMetrics();
      queryClient.invalidateQueries({ queryKey: ['device', id] });
    }
  }, [id, refetchMetrics, queryClient]);

  useWebSocket(handleWebSocketMessage);

  const copyToken = () => {
    const tokenToCopy = visibleToken || device?.token;
    if (tokenToCopy) {
      navigator.clipboard.writeText(tokenToCopy);
      setCopiedToken(true);
      toast.success(t.copyToken);
      setTimeout(() => setCopiedToken(false), 3000);
    }
  };

  if (deviceLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="w-12 h-12 spinner" />
      </div>
    );
  }

  if (!device) {
    return (
      <div className="p-8 text-center">
        <p className="text-dark-400">{t.noDevicesFound}</p>
      </div>
    );
  }

  const latestMetric = metrics?.[metrics.length - 1];

  const chartData = {
    labels: metrics?.map((m: Metric) => format(new Date(m.created_at), 'HH:mm')) || [],
    datasets: [
      {
        label: `${t.temperature} (°C)`,
        // null/undefined = no data, Chart.js will skip these points
        data: metrics?.map((m: Metric) => m.temperature ?? null) || [],
        borderColor: '#f97316',
        backgroundColor: 'rgba(249, 115, 22, 0.1)',
        fill: true,
        tension: 0.4,
        spanGaps: false, // Don't connect line over null/missing values
      },
      {
        label: `${t.humidity} (%)`,
        data: metrics?.map((m: Metric) => m.humidity ?? null) || [],
        borderColor: '#06b6d4',
        backgroundColor: 'rgba(6, 182, 212, 0.1)',
        fill: true,
        tension: 0.4,
        spanGaps: false, // Don't connect line over null/missing values
      },
    ],
  };

  const chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: 'top' as const,
        labels: { color: '#9ca3af' },
      },
    },
    scales: {
      x: {
        grid: { color: 'rgba(255,255,255,0.05)' },
        ticks: { color: '#6b7280' },
      },
      y: {
        grid: { color: 'rgba(255,255,255,0.05)' },
        ticks: { color: '#6b7280' },
      },
    },
  };

  const periods = [
    { value: '1h', label: '1H' },
    { value: '6h', label: '6H' },
    { value: '24h', label: '24H' },
    { value: '168h', label: '7D' },
  ];

  const quickCommands = [
    { command: 'reboot', label: t.reboot, icon: Power },
    { command: 'toggle_dht', label: t.toggleDHT, icon: Thermometer },
    { command: 'toggle_mesh', label: t.toggleMesh, icon: Wifi },
  ];

  return (
    <div className="p-8">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        className="flex items-center gap-4 mb-8"
      >
        <button
          onClick={() => navigate('/devices')}
          className="p-2 rounded-lg hover:bg-dark-700 transition-colors"
        >
          <ArrowLeft className="w-5 h-5" />
        </button>
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-3xl font-bold">{device.name}</h1>
            <span className={`px-3 py-1 rounded-full text-xs font-medium ${
              device.is_online 
                ? 'bg-green-500/20 text-green-400' 
                : 'bg-red-500/20 text-red-400'
            }`}>
              {device.is_online ? t.online : 'Offline'}
            </span>
          </div>
          <p className="text-dark-400 text-sm mt-1">
            {device.platform} • {device.firmware} • {device.mac}
          </p>
        </div>
        <button
          onClick={() => {
            if (confirm(t.confirm + '?')) {
              deleteMutation.mutate();
            }
          }}
          className="p-2.5 rounded-lg hover:bg-red-500/20 text-red-400 transition-colors"
        >
          <Trash2 className="w-5 h-5" />
        </button>
      </motion.div>

      {/* Stats Grid */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
        {[
          { label: t.temperature, value: latestMetric?.temperature?.toFixed(1) ?? '--', unit: '°C', icon: Thermometer, color: 'text-orange-400' },
          { label: t.humidity, value: latestMetric?.humidity?.toFixed(1) ?? '--', unit: '%', icon: Droplets, color: 'text-cyan-400' },
          { label: t.wifiSignal, value: latestMetric?.rssi ?? '--', unit: 'dBm', icon: Wifi, color: 'text-purple-400' },
          { label: t.freeMemory, value: latestMetric?.free_heap ? (latestMetric.free_heap / 1024).toFixed(0) : '--', unit: 'KB', icon: Cpu, color: 'text-green-400' },
        ].map((stat, i) => (
          <motion.div
            key={stat.label}
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: i * 0.1 }}
            className="glass rounded-xl p-4"
          >
            <div className="flex items-center gap-2 mb-2">
              <stat.icon className={`w-4 h-4 ${stat.color}`} />
              <span className="text-sm text-dark-400">{stat.label}</span>
            </div>
            <p className="text-2xl font-bold">
              {stat.value}<span className="text-sm text-dark-500 ml-1">{stat.unit}</span>
            </p>
          </motion.div>
        ))}
      </div>

      {/* Chart */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.2 }}
        className="glass rounded-2xl p-6 mb-8"
      >
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">{t.sensorData}</h2>
          <div className="flex items-center gap-2">
            {periods.map((p) => (
              <button
                key={p.value}
                onClick={() => setSelectedPeriod(p.value)}
                className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                  selectedPeriod === p.value
                    ? 'bg-primary-500/20 text-primary-400'
                    : 'text-dark-400 hover:text-white hover:bg-dark-700'
                }`}
              >
                {p.label}
              </button>
            ))}
          </div>
        </div>
        <div className="h-72">
          <Line data={chartData} options={chartOptions} />
        </div>
      </motion.div>

      <div className="grid md:grid-cols-2 gap-6">
        {/* WiFi Networks */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.3 }}
          className="glass rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
            <Wifi className="w-5 h-5 text-primary-400" />
            {t.wifiNetworks}
          </h2>
          <div className="space-y-2 max-h-64 overflow-auto">
            {latestMetric?.wifi_scan?.slice(0, 10).map((network, i) => (
              <div key={i} className="flex items-center justify-between p-3 rounded-lg bg-dark-800/50">
                <div>
                  <p className="font-medium text-sm">{network.ssid || '(Hidden)'}</p>
                  <p className="text-xs text-dark-500">CH {network.channel} • {network.enc}</p>
                </div>
                <div className="text-right">
                  <p className={`text-sm font-mono ${
                    network.rssi > -50 ? 'text-green-400' : 
                    network.rssi > -70 ? 'text-yellow-400' : 'text-red-400'
                  }`}>{network.rssi} dBm</p>
                </div>
              </div>
            )) || <p className="text-dark-500 text-sm">{t.noDevicesFound}</p>}
          </div>
        </motion.div>

        {/* Quick Commands */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.4 }}
          className="glass rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
            <Send className="w-5 h-5 text-accent-400" />
            {t.quickCommands}
          </h2>
          <div className="grid grid-cols-3 gap-3 mb-4">
            {quickCommands.map((cmd) => (
              <button
                key={cmd.command}
                onClick={() => sendCommandMutation.mutate(cmd.command)}
                disabled={sendCommandMutation.isPending}
                className="flex flex-col items-center gap-2 p-4 rounded-xl bg-dark-800/50 hover:bg-dark-700 transition-colors disabled:opacity-50"
              >
                <cmd.icon className="w-5 h-5 text-primary-400" />
                <span className="text-xs">{cmd.label}</span>
              </button>
            ))}
          </div>
          
          <div className="pt-4 border-t border-dark-700/50">
            <h3 className="text-sm font-medium text-dark-400 mb-3">{t.recentCommands}</h3>
            <div className="space-y-2 max-h-32 overflow-auto">
              {commands?.slice(0, 5).map((cmd) => (
                <div key={cmd.id} className="flex items-center justify-between text-sm gap-2">
                  <span className="font-mono text-dark-300 flex-1">{cmd.command}</span>
                  <div className="flex items-center gap-2">
                    <span className={`px-2 py-0.5 rounded text-xs ${
                      cmd.status === 'acknowledged' ? 'bg-green-500/20 text-green-400' :
                      cmd.status === 'sent' ? 'bg-yellow-500/20 text-yellow-400' :
                      'bg-dark-600 text-dark-400'
                    }`}>{cmd.status}</span>
                    {cmd.status === 'pending' && (
                      <button
                        onClick={() => cancelCommandMutation.mutate(cmd.id)}
                        disabled={cancelCommandMutation.isPending}
                        className="p-1 rounded hover:bg-red-500/20 text-red-400 transition-colors"
                        title="Cancel command"
                      >
                        <X className="w-4 h-4" />
                      </button>
                    )}
                  </div>
                </div>
              )) || <p className="text-dark-500 text-sm">{t.noDevicesFound}</p>}
            </div>
          </div>
        </motion.div>

        {/* Alert Settings */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.5 }}
          className="glass rounded-2xl p-6"
        >
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold flex items-center gap-2">
              {device.alerts_enabled ? (
                <Bell className="w-5 h-5 text-green-400" />
              ) : (
                <BellOff className="w-5 h-5 text-dark-500" />
              )}
              Alerts
            </h2>
            <button
              onClick={() => updateAlertsMutation.mutate({ alerts_enabled: !device.alerts_enabled })}
              className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                device.alerts_enabled
                  ? 'bg-green-500/20 text-green-400 hover:bg-green-500/30'
                  : 'bg-dark-700 text-dark-400 hover:bg-dark-600'
              }`}
            >
              {device.alerts_enabled ? 'Enabled' : 'Disabled'}
            </button>
          </div>
          
          {device.alerts_enabled && (
            <div className="space-y-4">
              <div className="grid grid-cols-3 gap-4">
                <div>
                  <label className="block text-sm text-dark-400 mb-2">Min Temp (°C)</label>
                  <input
                    type="number"
                    step="0.5"
                    defaultValue={device.alert_temp_min ?? ''}
                    placeholder="0"
                    className="input-field text-sm"
                    onBlur={(e) => {
                      const val = e.target.value ? parseFloat(e.target.value) : undefined;
                      if (val !== device.alert_temp_min) {
                        updateAlertsMutation.mutate({ alert_temp_min: val });
                      }
                    }}
                  />
                </div>
                <div>
                  <label className="block text-sm text-dark-400 mb-2">Max Temp (°C)</label>
                  <input
                    type="number"
                    step="0.5"
                    defaultValue={device.alert_temp_max ?? 40}
                    placeholder="40"
                    className="input-field text-sm"
                    onBlur={(e) => {
                      const val = e.target.value ? parseFloat(e.target.value) : undefined;
                      if (val !== device.alert_temp_max) {
                        updateAlertsMutation.mutate({ alert_temp_max: val });
                      }
                    }}
                  />
                </div>
                <div>
                  <label className="block text-sm text-dark-400 mb-2">Max Humidity (%)</label>
                  <input
                    type="number"
                    step="1"
                    defaultValue={device.alert_humidity_max ?? 90}
                    placeholder="90"
                    className="input-field text-sm"
                    onBlur={(e) => {
                      const val = e.target.value ? parseFloat(e.target.value) : undefined;
                      if (val !== device.alert_humidity_max) {
                        updateAlertsMutation.mutate({ alert_humidity_max: val });
                      }
                    }}
                  />
                </div>
              </div>
              <p className="text-xs text-dark-500">
                Alerts will be logged and can trigger notifications via GCP Cloud Monitoring
              </p>
            </div>
          )}
        </motion.div>

        {/* Device Token */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.6 }}
          className="glass rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
            <Settings className="w-5 h-5 text-dark-400" />
            {t.deviceToken}
          </h2>
          {visibleToken ? (
            <div className="space-y-3">
              <div className="p-3 rounded-lg bg-yellow-500/10 border border-yellow-500/30 text-yellow-400 text-sm">
                ⚠️ {t.tokenWarning}
              </div>
              <div className="flex gap-3">
                <div className="relative flex-1">
                  <input
                    type="text"
                    value={visibleToken}
                    readOnly
                    className="input-field pr-12 font-mono text-sm"
                  />
                  <button
                    onClick={copyToken}
                    className="absolute right-3 top-1/2 -translate-y-1/2 p-1.5 rounded hover:bg-dark-600 transition-colors"
                  >
                    {copiedToken ? <Check className="w-4 h-4 text-green-400" /> : <Copy className="w-4 h-4 text-dark-400" />}
                  </button>
                </div>
                <button
                  onClick={() => setVisibleToken(null)}
                  className="btn-primary"
                >
                  {t.done}
                </button>
              </div>
            </div>
          ) : (
            <div className="flex gap-3">
              <div className="relative flex-1">
                <input
                  type="text"
                  value="••••••••••••••••••••••••••••••••"
                  readOnly
                  className="input-field font-mono text-sm text-dark-500"
                />
              </div>
              <button
                onClick={() => {
                  if (confirm(t.confirm + '?')) {
                    regenerateTokenMutation.mutate();
                  }
                }}
                disabled={regenerateTokenMutation.isPending}
                className="btn-secondary flex items-center gap-2"
              >
                <RefreshCw className={`w-4 h-4 ${regenerateTokenMutation.isPending ? 'animate-spin' : ''}`} />
                {t.regenerate}
              </button>
            </div>
          )}
        </motion.div>
      </div>
    </div>
  );
}
