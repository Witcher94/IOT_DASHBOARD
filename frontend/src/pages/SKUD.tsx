import { useState, useEffect, useRef, useCallback } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { motion, AnimatePresence } from 'framer-motion';
import {
  CreditCard,
  FileText,
  Trash2,
  CheckCircle,
  Clock,
  XCircle,
  Filter,
  Wifi,
  WifiOff,
  Search,
  RotateCcw,
  Cpu,
  Pencil,
  Check,
  X,
} from 'lucide-react';
import toast from 'react-hot-toast';
import { skudApi, devicesApi } from '../services/api';
import { useTranslation } from '../contexts/settingsStore';
import { useAuthStore } from '../contexts/authStore';
import type { CardStatus, AccessLog, Device } from '../types';

type TabType = 'cards' | 'logs';

const statusColors: Record<CardStatus, string> = {
  pending: 'bg-amber-500/15 text-amber-300 border-amber-500/30',
  active: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
  disabled: 'bg-rose-500/15 text-rose-300 border-rose-500/30',
};

const statusIcons: Record<CardStatus, React.ReactNode> = {
  pending: <Clock className="w-4 h-4" />,
  active: <CheckCircle className="w-4 h-4" />,
  disabled: <XCircle className="w-4 h-4" />,
};

function StatusBadge({ status }: { status: CardStatus }) {
  const t = useTranslation();
  const labels: Record<CardStatus, string> = {
    pending: t.pending,
    active: t.active,
    disabled: t.disabled,
  };

  return (
    <span className={`inline-flex items-center gap-1.5 px-3 py-1 text-xs font-medium rounded-full border ${statusColors[status]}`}>
      {statusIcons[status]}
      {labels[status]}
    </span>
  );
}

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleString();
}

// WebSocket URL helper
function getWsUrl(ticket: string): string {
  const apiUrl = import.meta.env.VITE_API_URL || `${window.location.origin}/api/v1`;
  const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = apiUrl.replace(/^https?:/, wsProtocol);
  return `${wsUrl}/ws?ticket=${ticket}`;
}

export default function SKUD() {
  const t = useTranslation();
  const { token } = useAuthStore();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  
  const [activeTab, setActiveTab] = useState<TabType>('cards');
  const [selectedDeviceId, setSelectedDeviceId] = useState<string>(searchParams.get('device') || '');
  const [statusFilter, setStatusFilter] = useState<CardStatus | ''>('');
  
  // Log filters
  const [logActionFilter, setLogActionFilter] = useState<string>('');
  const [logAllowedFilter, setLogAllowedFilter] = useState<string>(''); // '', 'true', 'false'
  const [logCardUidFilter, setLogCardUidFilter] = useState<string>('');
  
  // WebSocket state for logs
  const [wsConnected, setWsConnected] = useState(false);
  const [wsLogs, setWsLogs] = useState<AccessLog[]>([]);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<number | null>(null);

  // Fetch SKUD devices
  const { data: allDevices } = useQuery({
    queryKey: ['devices'],
    queryFn: devicesApi.getAll,
  });

  // Filter only SKUD devices
  const skudDevices = allDevices?.filter((d: Device) => d.device_type === 'skud') || [];

  // Update URL when device changes
  const handleDeviceChange = (deviceId: string) => {
    setSelectedDeviceId(deviceId);
    if (deviceId) {
      setSearchParams({ device: deviceId });
    } else {
      setSearchParams({});
    }
  };


  // Load logs with filters
  const loadLogs = useCallback(async () => {
    try {
      const filters: Parameters<typeof skudApi.getAccessLogs>[0] = {
        limit: 100,
      };
      if (selectedDeviceId) filters.device_id = selectedDeviceId;
      if (logActionFilter) filters.action = logActionFilter;
      if (logAllowedFilter === 'true') filters.allowed = true;
      if (logAllowedFilter === 'false') filters.allowed = false;
      if (logCardUidFilter) filters.card_uid = logCardUidFilter;
      
      const logs = await skudApi.getAccessLogs(filters);
      setWsLogs(logs);
    } catch (e) {
      console.error('[SKUD] Failed to load logs:', e);
    }
  }, [logActionFilter, logAllowedFilter, logCardUidFilter, selectedDeviceId]);

  // Connect to WebSocket for real-time logs
  const connectWebSocket = useCallback(async () => {
    if (!token) return;

    try {
      // Get WebSocket ticket first
      const apiUrl = import.meta.env.VITE_API_URL || `${window.location.origin}/api/v1`;
      const response = await fetch(`${apiUrl}/ws/ticket`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
      });
      
      if (!response.ok) {
        console.error('[SKUD WS] Failed to get ticket');
        return;
      }
      
      const { ticket } = await response.json();
      const wsUrl = getWsUrl(ticket);
      
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = async () => {
        console.log('[SKUD WS] Connected');
        setWsConnected(true);
        
        // Load initial logs from REST API with filters
        await loadLogs();
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (data.type === 'access_log' && data.data) {
            // New access log received
            setWsLogs((prev) => [data.data as AccessLog, ...prev].slice(0, 100)); // Keep last 100
          } else if (data.type === 'card_update' && data.data) {
            // Card created/updated/deleted - refresh cards list
            console.log('[SKUD WS] Card update:', data.data.action);
            queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
          }
        } catch (e) {
          console.error('[SKUD WS] Parse error:', e);
        }
      };

      ws.onerror = (error) => {
        console.error('[SKUD WS] Error:', error);
      };

      ws.onclose = () => {
        console.log('[SKUD WS] Disconnected');
        setWsConnected(false);
        wsRef.current = null;
        
        // Reconnect after 3 seconds
        reconnectTimeoutRef.current = window.setTimeout(() => {
          connectWebSocket();
        }, 3000);
      };
    } catch (error) {
      console.error('[SKUD WS] Connection error:', error);
    }
  }, [token, activeTab, loadLogs]);

  // Reload logs when filters change
  useEffect(() => {
    if (activeTab === 'logs' && wsConnected) {
      loadLogs();
    }
  }, [activeTab, wsConnected, loadLogs]);

  // Connect WebSocket on mount (for both cards and logs real-time updates)
  useEffect(() => {
    connectWebSocket();

    return () => {
      if (wsRef.current) {
        wsRef.current.close();
      }
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
    };
  }, [connectWebSocket]);

  // Queries - get ALL cards (show linked devices for each)
  const { data: cards, isLoading: cardsLoading } = useQuery({
    queryKey: ['skud-cards', statusFilter],
    queryFn: () => skudApi.getCards(statusFilter || undefined),
    enabled: activeTab === 'cards',
  });

  // Mutations
  const updateCardStatusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: CardStatus }) =>
      skudApi.updateCardStatus(id, { status }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
      toast.success(t.cardStatusUpdated);
    },
    onError: () => toast.error(t.error),
  });

  const deleteCardMutation = useMutation({
    mutationFn: skudApi.deleteCard,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
      toast.success(t.delete);
    },
    onError: () => toast.error(t.error),
  });

  const linkCardMutation = useMutation({
    mutationFn: ({ cardId, deviceId }: { cardId: string; deviceId: string }) =>
      skudApi.linkCardToDevice(cardId, deviceId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
      toast.success(t.linkCard);
    },
    onError: () => toast.error(t.error),
  });

  const unlinkCardMutation = useMutation({
    mutationFn: ({ cardId, deviceId }: { cardId: string; deviceId: string }) =>
      skudApi.unlinkCardFromDevice(cardId, deviceId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
      toast.success(t.unlinkCard);
    },
    onError: () => toast.error(t.error),
  });

  const updateCardMutation = useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) =>
      skudApi.updateCard(id, { name }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
      toast.success(t.cardUpdated || 'Картку оновлено');
    },
    onError: () => toast.error(t.error),
  });

  // State for inline editing
  const [editingCardId, setEditingCardId] = useState<string | null>(null);
  const [editingName, setEditingName] = useState('');

  const startEditing = (cardId: string, currentName: string) => {
    setEditingCardId(cardId);
    setEditingName(currentName);
  };

  const saveCardName = (cardId: string) => {
    updateCardMutation.mutate({ id: cardId, name: editingName });
    setEditingCardId(null);
  };

  const cancelEditing = () => {
    setEditingCardId(null);
    setEditingName('');
  };

  const tabs = [
    { id: 'cards' as const, icon: CreditCard, label: t.cards },
    { id: 'logs' as const, icon: FileText, label: t.accessLogs },
  ];

  return (
    <div className="p-8">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        className="mb-8"
      >
        <h1 className="text-3xl font-bold mb-2">
          <span className="gradient-text">SKUD</span>
        </h1>
        <p className="text-dark-400">{t.accessControl || 'Access Control System'}</p>
      </motion.div>

      {/* Tabs */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.1 }}
        className="flex gap-2 mb-6 p-1 bg-dark-800/50 rounded-xl w-fit"
      >
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`flex items-center gap-2 px-4 py-2.5 rounded-lg font-medium transition-all ${
              activeTab === tab.id
                ? 'bg-gradient-to-r from-primary-500 to-accent-400 text-dark-950'
                : 'text-dark-300 hover:text-white hover:bg-dark-700'
            }`}
          >
            <tab.icon className="w-4 h-4" />
            {tab.label}
            {tab.id === 'logs' && (
              <span className={`w-2 h-2 rounded-full ${wsConnected ? 'bg-emerald-400' : 'bg-rose-400'}`} />
            )}
          </button>
        ))}
      </motion.div>

      {/* Cards Tab */}
      {activeTab === 'cards' && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.2 }}
        >
          {/* Filter */}
          <div className="flex items-center gap-4 mb-6">
            <div className="flex items-center gap-2">
              <Filter className="w-4 h-4 text-dark-400" />
              <select
                value={statusFilter}
                onChange={(e) => setStatusFilter(e.target.value as CardStatus | '')}
                className="input-field w-auto py-2"
              >
                <option value="">{t.allCards}</option>
                <option value="pending">{t.pending}</option>
                <option value="active">{t.active}</option>
                <option value="disabled">{t.disabled}</option>
              </select>
            </div>
          </div>

          {/* Cards Grid */}
          {cardsLoading ? (
            <div className="flex items-center justify-center py-20">
              <div className="w-12 h-12 spinner" />
            </div>
          ) : cards && cards.length > 0 ? (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {cards.map((card, index) => (
                <motion.div
                  key={card.id}
                  initial={{ opacity: 0, y: 20 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: index * 0.05 }}
                  className="glass rounded-xl p-5 border border-dark-700/50 hover:border-primary-500/30 transition-all"
                >
                  <div className="flex items-start justify-between mb-4">
                    <div className="flex-1 min-w-0">
                      {/* Editable Name */}
                      {editingCardId === card.id ? (
                        <div className="flex items-center gap-2 mb-2">
                          <input
                            type="text"
                            value={editingName}
                            onChange={(e) => setEditingName(e.target.value)}
                            placeholder="Ім'я картки..."
                            className="input-field text-lg font-semibold py-1 flex-1"
                            autoFocus
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') saveCardName(card.id);
                              if (e.key === 'Escape') cancelEditing();
                            }}
                          />
                          <button
                            onClick={() => saveCardName(card.id)}
                            className="p-1.5 rounded-lg bg-emerald-500/20 text-emerald-300 hover:bg-emerald-500/30 transition-colors"
                          >
                            <Check className="w-4 h-4" />
                          </button>
                          <button
                            onClick={cancelEditing}
                            className="p-1.5 rounded-lg bg-dark-700 text-dark-300 hover:bg-dark-600 transition-colors"
                          >
                            <X className="w-4 h-4" />
                          </button>
                        </div>
                      ) : (
                        <div className="flex items-center gap-2 mb-2 group">
                          <p className="text-lg font-semibold text-white truncate">
                            {card.name || <span className="text-dark-400 italic">Без імені</span>}
                          </p>
                          <button
                            onClick={() => startEditing(card.id, card.name || '')}
                            className="p-1 rounded-lg text-dark-500 hover:text-primary-400 hover:bg-dark-700 transition-colors opacity-0 group-hover:opacity-100"
                          >
                            <Pencil className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      )}
                      {/* Card UID (always visible) */}
                      <p className="font-mono text-xs text-dark-400 mb-2 truncate">
                        UID: {card.card_uid}
                      </p>
                      <StatusBadge status={card.status} />
                    </div>
                    <button
                      onClick={() => {
                        if (confirm(t.confirm + '?')) {
                          deleteCardMutation.mutate(card.id);
                        }
                      }}
                      className="p-2 rounded-lg text-dark-400 hover:text-rose-400 hover:bg-rose-500/10 transition-colors flex-shrink-0"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>

                  {/* Device Links Manager */}
                  <div className="mb-4">
                    <p className="text-xs text-dark-400 mb-2">{t.linkedDevices}:</p>
                    <div className="flex flex-wrap gap-2">
                      {skudDevices.map((device: Device) => {
                        const isLinked = card.devices?.some((d) => d.id === device.id);
                        return (
                          <button
                            key={device.id}
                            onClick={() => {
                              if (isLinked) {
                                unlinkCardMutation.mutate({ cardId: card.id, deviceId: device.id });
                              } else {
                                linkCardMutation.mutate({ cardId: card.id, deviceId: device.id });
                              }
                            }}
                            disabled={linkCardMutation.isPending || unlinkCardMutation.isPending}
                            className={`px-2 py-1 text-xs rounded-lg transition-all flex items-center gap-1 ${
                              isLinked
                                ? 'bg-primary-500/20 text-primary-300 border border-primary-500/30 hover:bg-rose-500/20 hover:text-rose-300 hover:border-rose-500/30'
                                : 'bg-dark-700/50 text-dark-400 border border-transparent hover:bg-emerald-500/20 hover:text-emerald-300 hover:border-emerald-500/30'
                            }`}
                          >
                            {isLinked ? '✓' : '+'} {device.name}
                          </button>
                        );
                      })}
                    </div>
                    {skudDevices.length === 0 && (
                      <p className="text-xs text-dark-500">{t.noSkudDevices}</p>
                    )}
                  </div>

                  {/* Status Actions */}
                  <div className="flex gap-2">
                    {card.status !== 'active' && (
                      <button
                        onClick={() => updateCardStatusMutation.mutate({ id: card.id, status: 'active' })}
                        className="flex-1 px-3 py-2 text-sm font-medium rounded-lg bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/20 transition-colors"
                      >
                        {t.active}
                      </button>
                    )}
                    {card.status !== 'disabled' && (
                      <button
                        onClick={() => updateCardStatusMutation.mutate({ id: card.id, status: 'disabled' })}
                        className="flex-1 px-3 py-2 text-sm font-medium rounded-lg bg-rose-500/10 text-rose-300 hover:bg-rose-500/20 transition-colors"
                      >
                        {t.disabled}
                      </button>
                    )}
                  </div>

                  <div className="mt-4 pt-4 border-t border-dark-700/50 text-xs text-dark-400">
                    {formatDate(card.updated_at)}
                  </div>
                </motion.div>
              ))}
            </div>
          ) : (
            <div className="text-center py-20">
              <CreditCard className="w-16 h-16 text-dark-500 mx-auto mb-4" />
              <p className="text-xl text-dark-300 mb-2">{t.noCardsFound}</p>
              <p className="text-dark-500">
                Картки з'являться автоматично при скануванні на пристрої СКУД
              </p>
            </div>
          )}
        </motion.div>
      )}

      {/* Logs Tab */}
      {activeTab === 'logs' && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.2 }}
        >
          {/* Filters */}
          <div className="glass rounded-xl p-4 mb-6">
            <div className="flex flex-wrap items-center gap-4">
              {/* Device filter */}
              <div className="flex items-center gap-2">
                <Cpu className="w-4 h-4 text-dark-400" />
                <select
                  value={selectedDeviceId}
                  onChange={(e) => handleDeviceChange(e.target.value)}
                  className="input-field w-auto py-2 text-sm"
                >
                  <option value="">Всі пристрої</option>
                  {skudDevices.map((device: Device) => (
                    <option key={device.id} value={device.id}>
                      {device.name} {device.is_online ? '🟢' : '🔴'}
                    </option>
                  ))}
                </select>
              </div>

              {/* Action filter */}
              <div className="flex items-center gap-2">
                <Filter className="w-4 h-4 text-dark-400" />
                <select
                  value={logActionFilter}
                  onChange={(e) => setLogActionFilter(e.target.value)}
                  className="input-field w-auto py-2 text-sm"
                >
                  <option value="">Всі дії</option>
                  <option value="verify">Верифікація</option>
                  <option value="register">Реєстрація</option>
                  <option value="card_status">Зміна статусу</option>
                  <option value="card_delete">Видалення</option>
                </select>
              </div>

              {/* Allowed filter */}
              <select
                value={logAllowedFilter}
                onChange={(e) => setLogAllowedFilter(e.target.value)}
                className="input-field w-auto py-2 text-sm"
              >
                <option value="">Всі результати</option>
                <option value="true">{t.allowed}</option>
                <option value="false">{t.denied}</option>
              </select>

              {/* Card UID search */}
              <div className="relative flex-1 min-w-[200px]">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-dark-400" />
                <input
                  type="text"
                  placeholder="Пошук за UID картки..."
                  value={logCardUidFilter}
                  onChange={(e) => setLogCardUidFilter(e.target.value)}
                  className="input-field pl-10 py-2 text-sm"
                />
              </div>

              {/* Reset filters */}
              {(logActionFilter || logAllowedFilter || logCardUidFilter) && (
                <button
                  onClick={() => {
                    setLogActionFilter('');
                    setLogAllowedFilter('');
                    setLogCardUidFilter('');
                  }}
                  className="flex items-center gap-2 px-3 py-2 text-sm text-dark-300 hover:text-white hover:bg-dark-700 rounded-lg transition-colors"
                >
                  <RotateCcw className="w-4 h-4" />
                  Скинути
                </button>
              )}

              {/* Connection status */}
              <div className={`ml-auto flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm ${
                wsConnected 
                  ? 'bg-emerald-500/10 text-emerald-300 border border-emerald-500/30' 
                  : 'bg-rose-500/10 text-rose-300 border border-rose-500/30'
              }`}>
                {wsConnected ? <Wifi className="w-4 h-4" /> : <WifiOff className="w-4 h-4" />}
                {wsConnected ? 'Live' : 'Offline'}
              </div>
            </div>
            
            <div className="mt-3 text-xs text-dark-400">
              Показано {wsLogs.length} {wsLogs.length === 1 ? 'запис' : wsLogs.length < 5 ? 'записи' : 'записів'}
            </div>
          </div>

          {/* Logs List */}
          {wsLogs.length > 0 ? (
            <div className="space-y-3">
              <AnimatePresence mode="popLayout">
                {wsLogs.map((log, index) => (
                  <motion.div
                    key={log.id}
                    initial={{ opacity: 0, x: -20, height: 0 }}
                    animate={{ opacity: 1, x: 0, height: 'auto' }}
                    exit={{ opacity: 0, x: 20, height: 0 }}
                    transition={{ delay: index * 0.02 }}
                    className={`glass rounded-xl p-4 border-l-4 ${
                      log.allowed ? 'border-l-emerald-500' : 'border-l-rose-500'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-4">
                        <span className={`inline-flex items-center gap-1.5 px-3 py-1 text-xs font-medium rounded-full ${
                          log.allowed ? 'bg-emerald-500/15 text-emerald-300' : 'bg-rose-500/15 text-rose-300'
                        }`}>
                          {log.allowed ? <CheckCircle className="w-3 h-3" /> : <XCircle className="w-3 h-3" />}
                          {log.allowed ? t.allowed : t.denied}
                        </span>
                        <span className="px-2 py-1 text-xs bg-dark-700/50 rounded-lg text-primary-300">
                          {log.action}
                        </span>
                        <span className="font-mono text-sm">{log.card_uid}</span>
                      </div>
                      <div className="flex items-center gap-4 text-sm text-dark-400">
                        <span>{log.device_id}</span>
                        <span>{formatDate(log.created_at)}</span>
                      </div>
                    </div>
                  </motion.div>
                ))}
              </AnimatePresence>
            </div>
          ) : (
            <div className="text-center py-20">
              <FileText className="w-16 h-16 text-dark-500 mx-auto mb-4" />
              <p className="text-xl text-dark-300">
                {wsConnected ? 'Очікування логів...' : 'Підключення...'}
              </p>
            </div>
          )}
        </motion.div>
      )}
    </div>
  );
}
