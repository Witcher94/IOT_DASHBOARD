import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { motion, AnimatePresence } from 'framer-motion';
import { Plus, Cpu, Search, X, Copy, Check, RefreshCw, CreditCard, Trash2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { devicesApi, metricsApi, skudApi } from '../services/api';
import { useTranslation } from '../contexts/settingsStore';
import DeviceCard from '../components/DeviceCard';

interface DeviceMetrics {
  temperature?: number;
  humidity?: number;
  rssi?: number;
  freeHeap?: number;
}

export default function Devices() {
  const t = useTranslation();
  const [showAddModal, setShowAddModal] = useState(false);
  const [newDeviceName, setNewDeviceName] = useState('');
  const [newDeviceToken, setNewDeviceToken] = useState<string | null>(null);
  const [copiedToken, setCopiedToken] = useState(false);
  const [search, setSearch] = useState('');
  const [deviceMetrics, setDeviceMetrics] = useState<Record<string, DeviceMetrics>>({});
  // SKUD devices
  const [showSkudModal, setShowSkudModal] = useState(false);
  const [newSkudDeviceId, setNewSkudDeviceId] = useState('');
  const [newSkudSecretKey, setNewSkudSecretKey] = useState('');
  const [newSkudDeviceName, setNewSkudDeviceName] = useState('');
  const queryClient = useQueryClient();

  const { data: devices, isLoading } = useQuery({
    queryKey: ['devices'],
    queryFn: devicesApi.getAll,
  });

  // Shared devices (devices shared with current user)
  const { data: sharedDevices } = useQuery({
    queryKey: ['shared-devices'],
    queryFn: devicesApi.getSharedWithMe,
  });

  // Load initial metrics for each device (including shared)
  useEffect(() => {
    const allDevices = [...(devices || []), ...(sharedDevices || [])];
    if (allDevices.length > 0) {
      allDevices.forEach(async (device) => {
        try {
          const metrics = await metricsApi.getByDeviceId(device.id, 1);
          if (metrics && metrics.length > 0) {
            const latest = metrics[0];
            setDeviceMetrics((prev) => ({
              ...prev,
              [device.id]: {
                temperature: latest.temperature ?? undefined,
                humidity: latest.humidity ?? undefined,
                rssi: latest.rssi ?? undefined,
                freeHeap: latest.free_heap ?? undefined,
              },
            }));
          }
        } catch {
          // Ignore errors
        }
      });
    }
  }, [devices, sharedDevices]);

  const createMutation = useMutation({
    mutationFn: devicesApi.create,
    onSuccess: (device) => {
      setNewDeviceToken(device.token);
      queryClient.invalidateQueries({ queryKey: ['devices'] });
      toast.success(t.deviceCreated);
    },
    onError: () => {
      toast.error(t.error);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: devicesApi.delete,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
      toast.success(t.delete);
    },
    onError: () => {
      toast.error(t.error);
    },
  });

  // SKUD Devices
  const { data: skudDevices } = useQuery({
    queryKey: ['skud-devices'],
    queryFn: skudApi.getAccessDevices,
  });

  const createSkudDeviceMutation = useMutation({
    mutationFn: skudApi.createAccessDevice,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['skud-devices'] });
      setShowSkudModal(false);
      setNewSkudDeviceId('');
      setNewSkudSecretKey('');
      setNewSkudDeviceName('');
      toast.success(t.deviceCreatedSkud);
    },
    onError: () => toast.error(t.error),
  });

  const deleteSkudDeviceMutation = useMutation({
    mutationFn: skudApi.deleteAccessDevice,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['skud-devices'] });
      toast.success(t.delete);
    },
    onError: () => toast.error(t.error),
  });

  const handleCreateSkudDevice = () => {
    if (newSkudDeviceId.trim() && newSkudSecretKey.trim()) {
      createSkudDeviceMutation.mutate({
        device_id: newSkudDeviceId.trim(),
        secret_key: newSkudSecretKey.trim(),
        name: newSkudDeviceName.trim() || undefined,
      });
    }
  };

  const handleCreate = () => {
    if (newDeviceName.trim()) {
      createMutation.mutate({ name: newDeviceName.trim() });
    }
  };

  const handleCloseModal = () => {
    setShowAddModal(false);
    setNewDeviceName('');
    setNewDeviceToken(null);
    setCopiedToken(false);
  };

  const copyToken = () => {
    if (newDeviceToken) {
      navigator.clipboard.writeText(newDeviceToken);
      setCopiedToken(true);
      toast.success(t.copyToken);
      setTimeout(() => setCopiedToken(false), 3000);
    }
  };

  // Filter: show only gateway and simple_device (hide mesh_node - they're shown in topology)
  // Hide devices that have gateway_id or mesh_node_id (they're mesh nodes shown in topology)
  const filteredDevices = devices?.filter((device) => {
    const matchesSearch = device.name.toLowerCase().includes(search.toLowerCase()) ||
      device.chip_id?.toLowerCase().includes(search.toLowerCase()) ||
      device.mac?.toLowerCase().includes(search.toLowerCase());
    
    // Hide mesh nodes (they have gateway_id or mesh_node_id)
    if (device.gateway_id || device.mesh_node_id) {
      return false;
    }
    
    // Show gateway and simple_device (or null device_type which defaults to simple_device)
    const isVisible = !device.device_type || device.device_type === 'simple_device' || device.device_type === 'gateway';
    return matchesSearch && isVisible;
  });

  return (
    <div className="p-8">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        className="flex items-center justify-between mb-8"
      >
        <div>
          <h1 className="text-3xl font-bold mb-2">
            <span className="gradient-text">{t.devices}</span>
          </h1>
          <p className="text-dark-400">
            {t.manageDevices}
          </p>
        </div>
        <button
          onClick={() => setShowAddModal(true)}
          className="btn-primary flex items-center gap-2"
        >
          <Plus className="w-5 h-5" />
          {t.addDevice}
        </button>
      </motion.div>

      {/* Search */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.1 }}
        className="relative mb-6"
      >
        <Search className="absolute left-4 top-1/2 -translate-y-1/2 w-5 h-5 text-dark-400" />
        <input
          type="text"
          placeholder={t.searchDevices}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="input-field pl-12"
        />
      </motion.div>

      {/* Devices Grid */}
      {isLoading ? (
        <div className="flex items-center justify-center py-20">
          <div className="w-12 h-12 spinner" />
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {filteredDevices?.map((device, index) => (
            <motion.div
              key={device.id}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: index * 0.05 }}
            >
              <DeviceCard
                device={device}
                isOnline={device.is_online}
                showActions
                temperature={deviceMetrics[device.id]?.temperature}
                humidity={deviceMetrics[device.id]?.humidity}
                rssi={deviceMetrics[device.id]?.rssi}
                freeHeap={deviceMetrics[device.id]?.freeHeap}
                onDelete={() => {
                  if (confirm(t.confirm + '?')) {
                    deleteMutation.mutate(device.id);
                  }
                }}
              />
            </motion.div>
          ))}
          {filteredDevices?.length === 0 && (
            <div className="col-span-full text-center py-20">
              <Cpu className="w-16 h-16 text-dark-500 mx-auto mb-4" />
              <p className="text-xl text-dark-300 mb-2">{t.noDevicesFound}</p>
              <p className="text-dark-500">
                {search ? t.searchDevices : t.addFirstDevice}
              </p>
            </div>
          )}
        </div>
      )}

      {/* Shared Devices Section */}
      {sharedDevices && sharedDevices.length > 0 && (
        <div className="mt-8">
          <h2 className="text-lg font-semibold mb-4 text-dark-300 flex items-center gap-2">
            <span className="text-primary-400">👥</span>
            Shared with me
          </h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {sharedDevices.map((device, index) => (
              <motion.div
                key={device.id}
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: index * 0.05 }}
              >
                <DeviceCard
                  device={device}
                  isOnline={device.is_online}
                  showActions={false}
                  temperature={deviceMetrics[device.id]?.temperature}
                  humidity={deviceMetrics[device.id]?.humidity}
                  rssi={deviceMetrics[device.id]?.rssi}
                  freeHeap={deviceMetrics[device.id]?.freeHeap}
                />
              </motion.div>
            ))}
          </div>
        </div>
      )}

      {/* SKUD Access Devices Section */}
      <div className="mt-10 pt-8 border-t border-dark-700/50">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-xl font-semibold flex items-center gap-3">
            <CreditCard className="w-6 h-6 text-primary-400" />
            <span>{t.accessDevices}</span>
          </h2>
          <button
            onClick={() => setShowSkudModal(true)}
            className="btn-secondary flex items-center gap-2"
          >
            <Plus className="w-4 h-4" />
            {t.addAccessDevice}
          </button>
        </div>

        {skudDevices && skudDevices.length > 0 ? (
          <div className="glass rounded-xl overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-dark-700/50">
                  <th className="text-left px-6 py-4 text-sm font-semibold text-dark-300">{t.deviceId}</th>
                  <th className="text-left px-6 py-4 text-sm font-semibold text-dark-300">{t.secretKey}</th>
                  <th className="text-left px-6 py-4 text-sm font-semibold text-dark-300">Name</th>
                  <th className="px-6 py-4"></th>
                </tr>
              </thead>
              <tbody>
                {skudDevices.map((device) => (
                  <tr key={device.id} className="border-b border-dark-700/30 hover:bg-dark-800/30">
                    <td className="px-6 py-4 font-mono text-sm">{device.device_id}</td>
                    <td className="px-6 py-4 font-mono text-sm text-dark-400">{device.secret_key}</td>
                    <td className="px-6 py-4 text-sm">{device.name || '-'}</td>
                    <td className="px-6 py-4">
                      <button
                        onClick={() => {
                          if (confirm(t.confirm + '?')) {
                            deleteSkudDeviceMutation.mutate(device.id);
                          }
                        }}
                        className="p-2 rounded-lg text-dark-400 hover:text-rose-400 hover:bg-rose-500/10 transition-colors"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="text-center py-12 glass rounded-xl">
            <CreditCard className="w-12 h-12 text-dark-500 mx-auto mb-3" />
            <p className="text-dark-300">{t.noAccessDevices}</p>
          </div>
        )}
      </div>

      {/* Add Device Modal */}
      <AnimatePresence>
        {showAddModal && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
            onClick={handleCloseModal}
          >
            <motion.div
              initial={{ scale: 0.9, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              exit={{ scale: 0.9, opacity: 0 }}
              onClick={(e) => e.stopPropagation()}
              className="w-full max-w-md glass rounded-2xl p-6"
            >
              <div className="flex items-center justify-between mb-6">
                <h2 className="text-xl font-semibold">
                  {newDeviceToken ? t.deviceCreated : t.addDevice}
                </h2>
                <button
                  onClick={handleCloseModal}
                  className="p-2 rounded-lg hover:bg-dark-700 transition-colors"
                >
                  <X className="w-5 h-5" />
                </button>
              </div>

              {!newDeviceToken ? (
                <>
                  <div className="mb-6">
                    <label className="block text-sm text-dark-400 mb-2">{t.devices}</label>
                    <input
                      type="text"
                      placeholder="Living Room Sensor"
                      value={newDeviceName}
                      onChange={(e) => setNewDeviceName(e.target.value)}
                      className="input-field"
                      autoFocus
                    />
                  </div>
                  <div className="flex gap-3">
                    <button onClick={handleCloseModal} className="btn-secondary flex-1">
                      {t.cancel}
                    </button>
                    <button
                      onClick={handleCreate}
                      disabled={!newDeviceName.trim() || createMutation.isPending}
                      className="btn-primary flex-1 disabled:opacity-50 flex items-center justify-center gap-2"
                    >
                      {createMutation.isPending && <RefreshCw className="w-4 h-4 animate-spin" />}
                      {t.addDevice}
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <div className="mb-6">
                    <p className="text-dark-300 mb-4">
                      {t.tokenWarning}
                    </p>
                    <div className="relative">
                      <input
                        type="text"
                        value={newDeviceToken}
                        readOnly
                        className="input-field pr-12 font-mono text-sm"
                      />
                      <button
                        onClick={copyToken}
                        className="absolute right-3 top-1/2 -translate-y-1/2 p-1.5 rounded-lg hover:bg-dark-600 transition-colors"
                      >
                        {copiedToken ? (
                          <Check className="w-5 h-5 text-green-400" />
                        ) : (
                          <Copy className="w-5 h-5 text-dark-400" />
                        )}
                      </button>
                    </div>
                  </div>
                  <div className="p-4 rounded-xl bg-amber-500/10 border border-amber-500/20 mb-6">
                    <p className="text-sm text-amber-200">
                      ⚠️ {t.tokenWarning}
                    </p>
                  </div>
                  <button onClick={handleCloseModal} className="btn-primary w-full">
                    {t.done}
                  </button>
                </>
              )}
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Add SKUD Device Modal */}
      <AnimatePresence>
        {showSkudModal && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
            onClick={() => setShowSkudModal(false)}
          >
            <motion.div
              initial={{ scale: 0.9, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              exit={{ scale: 0.9, opacity: 0 }}
              onClick={(e) => e.stopPropagation()}
              className="w-full max-w-md glass rounded-2xl p-6"
            >
              <div className="flex items-center justify-between mb-6">
                <h2 className="text-xl font-semibold">{t.addAccessDevice}</h2>
                <button
                  onClick={() => setShowSkudModal(false)}
                  className="p-2 rounded-lg hover:bg-dark-700 transition-colors"
                >
                  <X className="w-5 h-5" />
                </button>
              </div>

              <div className="space-y-4 mb-6">
                <div>
                  <label className="block text-sm text-dark-400 mb-2">{t.deviceId} *</label>
                  <input
                    type="text"
                    placeholder="0000E8BA66F4E9D4"
                    value={newSkudDeviceId}
                    onChange={(e) => setNewSkudDeviceId(e.target.value)}
                    className="input-field font-mono"
                    autoFocus
                  />
                </div>
                <div>
                  <label className="block text-sm text-dark-400 mb-2">{t.secretKey} *</label>
                  <input
                    type="text"
                    placeholder="FIRST_ESP_SECRET"
                    value={newSkudSecretKey}
                    onChange={(e) => setNewSkudSecretKey(e.target.value)}
                    className="input-field font-mono"
                  />
                </div>
                <div>
                  <label className="block text-sm text-dark-400 mb-2">Name (optional)</label>
                  <input
                    type="text"
                    placeholder="Main Entrance Reader"
                    value={newSkudDeviceName}
                    onChange={(e) => setNewSkudDeviceName(e.target.value)}
                    className="input-field"
                  />
                </div>
              </div>

              <div className="flex gap-3">
                <button onClick={() => setShowSkudModal(false)} className="btn-secondary flex-1">
                  {t.cancel}
                </button>
                <button
                  onClick={handleCreateSkudDevice}
                  disabled={!newSkudDeviceId.trim() || !newSkudSecretKey.trim() || createSkudDeviceMutation.isPending}
                  className="btn-primary flex-1 disabled:opacity-50"
                >
                  {createSkudDeviceMutation.isPending ? t.loading : t.addAccessDevice}
                </button>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
