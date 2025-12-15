import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { useSearchParams, Link } from 'react-router-dom';
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
  Key,
  Shield,
  UserPlus,
  RefreshCw,
  Calendar,
  Tag,
  List,
  GitBranch,
  ChevronDown,
  ChevronRight,
} from 'lucide-react';
import toast from 'react-hot-toast';
import { skudApi, devicesApi } from '../services/api';
import { useTranslation } from '../contexts/settingsStore';
import { useAuthStore } from '../contexts/authStore';
import type { CardStatus, AccessLog, Device, AccessLogAction, CardType } from '../types';

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

// Action labels and icons for logs
const actionConfig: Record<AccessLogAction | string, { label: string; icon: React.ReactNode; color: string }> = {
  verify: { label: 'Верифікація', icon: <Shield className="w-3.5 h-3.5" />, color: 'bg-blue-500/15 text-blue-300 border-blue-500/30' },
  register: { label: 'Реєстрація', icon: <UserPlus className="w-3.5 h-3.5" />, color: 'bg-violet-500/15 text-violet-300 border-violet-500/30' },
  card_status: { label: 'Зміна статусу', icon: <RefreshCw className="w-3.5 h-3.5" />, color: 'bg-amber-500/15 text-amber-300 border-amber-500/30' },
  card_delete: { label: 'Видалення', icon: <Trash2 className="w-3.5 h-3.5" />, color: 'bg-rose-500/15 text-rose-300 border-rose-500/30' },
  desfire_auth: { label: 'DESFire Auth', icon: <Key className="w-3.5 h-3.5" />, color: 'bg-cyan-500/15 text-cyan-300 border-cyan-500/30' },
  provision: { label: 'Провізійування', icon: <CreditCard className="w-3.5 h-3.5" />, color: 'bg-indigo-500/15 text-indigo-300 border-indigo-500/30' },
  key_rotation: { label: 'Ротація ключа', icon: <Key className="w-3.5 h-3.5" />, color: 'bg-pink-500/15 text-pink-300 border-pink-500/30' },
  clone_attempt: { label: '⚠️ Клон!', icon: <Shield className="w-3.5 h-3.5" />, color: 'bg-red-600/30 text-red-300 border-red-500/50' },
};

// Card type labels and config
const cardTypeLabels: Record<CardType | string, string> = {
  MIFARE_CLASSIC_1K: 'MIFARE Classic 1K',
  MIFARE_CLASSIC_4K: 'MIFARE Classic 4K',
  MIFARE_DESFIRE: 'MIFARE DESFire',
  MIFARE_ULTRALIGHT: 'MIFARE Ultralight',
  UNKNOWN: 'Невідомий',
};

// Card type icons and colors
const cardTypeConfig: Record<string, { icon: React.ReactNode; color: string; bgColor: string }> = {
  MIFARE_CLASSIC_1K: { icon: <CreditCard className="w-4 h-4" />, color: 'text-blue-400', bgColor: 'bg-blue-500/10' },
  MIFARE_CLASSIC_4K: { icon: <CreditCard className="w-4 h-4" />, color: 'text-blue-400', bgColor: 'bg-blue-500/10' },
  MIFARE_DESFIRE: { icon: <Shield className="w-4 h-4" />, color: 'text-cyan-400', bgColor: 'bg-cyan-500/10' },
  MIFARE_ULTRALIGHT: { icon: <CreditCard className="w-4 h-4" />, color: 'text-gray-400', bgColor: 'bg-gray-500/10' },
  UNKNOWN: { icon: <CreditCard className="w-4 h-4" />, color: 'text-dark-400', bgColor: 'bg-dark-600/50' },
};

// Scenarios by card type - what actions are possible
const cardTypeScenarios: Record<string, string[]> = {
  MIFARE_CLASSIC_1K: ['register', 'verify', 'card_status', 'card_delete'],
  MIFARE_CLASSIC_4K: ['register', 'verify', 'card_status', 'card_delete'],
  MIFARE_DESFIRE: ['register', 'provision', 'desfire_auth', 'verify', 'key_rotation', 'card_status', 'card_delete'],
  MIFARE_ULTRALIGHT: ['register', 'verify', 'card_status', 'card_delete'],
  UNKNOWN: ['register', 'verify', 'card_status', 'card_delete'],
};

// Group logs by card_uid for tree view
interface LogGroup {
  card_uid: string;
  card_name: string; // Display name for the card
  card_type: string;
  logs: AccessLog[];
  lastAction: string;
  lastTime: string;
  hasErrors: boolean;
  successCount: number;
  failCount: number;
}

function groupLogsByCard(logs: AccessLog[]): LogGroup[] {
  const groups: Record<string, LogGroup> = {};
  
  // Group logs by card_uid
  for (const log of logs) {
    if (!groups[log.card_uid]) {
      groups[log.card_uid] = {
        card_uid: log.card_uid,
        card_name: log.card_name || '', // Use card_name from first log
        card_type: log.card_type || 'UNKNOWN',
        logs: [],
        lastAction: log.action,
        lastTime: log.created_at,
        hasErrors: false,
        successCount: 0,
        failCount: 0,
      };
    }
    // Update card_name if we find a non-empty one (in case first log didn't have it)
    if (log.card_name && !groups[log.card_uid].card_name) {
      groups[log.card_uid].card_name = log.card_name;
    }
    groups[log.card_uid].logs.push(log);
    if (!log.allowed) groups[log.card_uid].hasErrors = true;
    if (log.allowed) groups[log.card_uid].successCount++;
    else groups[log.card_uid].failCount++;
  }
  
  // Sort groups by last activity, and logs within groups by time (newest first)
  return Object.values(groups)
    .map(group => ({
      ...group,
      logs: group.logs.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()),
    }))
    .sort((a, b) => new Date(b.lastTime).getTime() - new Date(a.lastTime).getTime());
}

type LogViewMode = 'list' | 'tree';

function ActionBadge({ action }: { action: string }) {
  const config = actionConfig[action] || { label: action, icon: <FileText className="w-3.5 h-3.5" />, color: 'bg-gray-500/15 text-gray-300 border-gray-500/30' };
  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg border ${config.color}`}>
      {config.icon}
      {config.label}
    </span>
  );
}

function CardTypeBadge({ cardType }: { cardType: string }) {
  const label = cardTypeLabels[cardType] || cardType || 'Невідомий';
  const isDESFire = cardType === 'MIFARE_DESFIRE';
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 text-xs rounded-md ${
      isDESFire ? 'bg-cyan-500/10 text-cyan-400' : 'bg-dark-600/50 text-dark-300'
    }`}>
      <Tag className="w-3 h-3" />
      {label}
    </span>
  );
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
  const [searchParams, setSearchParams] = useSearchParams();
  
  const [activeTab, setActiveTab] = useState<TabType>('cards');
  const [selectedDeviceId, setSelectedDeviceId] = useState<string>(searchParams.get('device') || '');
  const [statusFilter, setStatusFilter] = useState<CardStatus | ''>('');
  
  // Log filters
  const [logActionFilter, setLogActionFilter] = useState<string>('');
  const [logAllowedFilter, setLogAllowedFilter] = useState<string>(''); // '', 'true', 'false'
  const [logCardUidFilter, setLogCardUidFilter] = useState<string>('');
  const [logCardTypeFilter, setLogCardTypeFilter] = useState<string>('');
  const [logFromDate, setLogFromDate] = useState<string>('');
  const [logToDate, setLogToDate] = useState<string>('');
  const [logViewMode, setLogViewMode] = useState<LogViewMode>('list');
  const [expandedCards, setExpandedCards] = useState<Set<string>>(new Set());

  // Toggle card expansion in tree view
  const toggleCardExpanded = (cardUid: string) => {
    setExpandedCards(prev => {
      const next = new Set(prev);
      if (next.has(cardUid)) next.delete(cardUid);
      else next.add(cardUid);
      return next;
    });
  };
  
  // WebSocket state for logs
  const [wsConnected, setWsConnected] = useState(false);
  const [wsLogs, setWsLogs] = useState<AccessLog[]>([]);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<number | null>(null);

  // Grouped logs for tree view (must be after wsLogs declaration)
  const groupedLogs = useMemo(() => groupLogsByCard(wsLogs), [wsLogs]);

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
      if (logCardTypeFilter) filters.card_type = logCardTypeFilter;
      if (logFromDate) filters.from_date = logFromDate;
      if (logToDate) filters.to_date = logToDate;
      
      const logs = await skudApi.getAccessLogs(filters);
      setWsLogs(logs);
    } catch (e) {
      console.error('[SKUD] Failed to load logs:', e);
    }
  }, [logActionFilter, logAllowedFilter, logCardUidFilter, logCardTypeFilter, logFromDate, logToDate, selectedDeviceId]);

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
      toast.success(t.cardStatusUpdated);
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
                      {/* Card UID and Type */}
                      <Link 
                        to={`/skud/cards/${card.id}`}
                        className="font-mono text-xs text-dark-400 mb-2 truncate block hover:text-primary-400 transition-colors"
                      >
                        UID: {card.card_uid}
                      </Link>
                      <div className="flex items-center gap-2 flex-wrap">
                        <StatusBadge status={card.status} />
                        {card.card_type && (
                          <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-dark-700/50 text-dark-300 border border-dark-600">
                            {card.card_type.replace(/_/g, ' ')}
                          </span>
                        )}
                      </div>
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
            {/* Row 1: Main filters */}
            <div className="flex flex-wrap items-center gap-3 mb-3">
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

              {/* Action filter - All 7 action types */}
              <div className="flex items-center gap-2">
                <Filter className="w-4 h-4 text-dark-400" />
                <select
                  value={logActionFilter}
                  onChange={(e) => setLogActionFilter(e.target.value)}
                  className="input-field w-auto py-2 text-sm"
                >
                  <option value="">Всі дії</option>
                  <option value="verify">🛡️ Верифікація</option>
                  <option value="register">👤 Реєстрація</option>
                  <option value="card_status">🔄 Зміна статусу</option>
                  <option value="card_delete">🗑️ Видалення</option>
                  <option value="desfire_auth">🔐 DESFire Auth</option>
                  <option value="provision">💳 Провізійування</option>
                  <option value="key_rotation">🔑 Ротація ключа</option>
                  <option value="clone_attempt">⚠️ Клон пристрою</option>
                </select>
              </div>

              {/* Result filter */}
              <select
                value={logAllowedFilter}
                onChange={(e) => setLogAllowedFilter(e.target.value)}
                className="input-field w-auto py-2 text-sm"
              >
                <option value="">Всі результати</option>
                <option value="true">✅ {t.allowed}</option>
                <option value="false">❌ {t.denied}</option>
              </select>

              {/* Card type filter */}
              <div className="flex items-center gap-2">
                <Tag className="w-4 h-4 text-dark-400" />
                <select
                  value={logCardTypeFilter}
                  onChange={(e) => setLogCardTypeFilter(e.target.value)}
                  className="input-field w-auto py-2 text-sm"
                >
                  <option value="">Всі типи карток</option>
                  <option value="MIFARE_CLASSIC_1K">MIFARE Classic 1K</option>
                  <option value="MIFARE_CLASSIC_4K">MIFARE Classic 4K</option>
                  <option value="MIFARE_DESFIRE">MIFARE DESFire</option>
                  <option value="MIFARE_ULTRALIGHT">MIFARE Ultralight</option>
                </select>
              </div>

              {/* View mode toggle */}
              <div className="flex items-center gap-1 bg-dark-700/50 rounded-lg p-1">
                <button
                  onClick={() => setLogViewMode('list')}
                  className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md transition-colors ${
                    logViewMode === 'list' 
                      ? 'bg-primary-500/20 text-primary-300' 
                      : 'text-dark-400 hover:text-dark-200'
                  }`}
                  title="Список"
                >
                  <List className="w-4 h-4" />
                </button>
                <button
                  onClick={() => setLogViewMode('tree')}
                  className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md transition-colors ${
                    logViewMode === 'tree' 
                      ? 'bg-primary-500/20 text-primary-300' 
                      : 'text-dark-400 hover:text-dark-200'
                  }`}
                  title="Дерево по картках"
                >
                  <GitBranch className="w-4 h-4" />
                </button>
              </div>

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

            {/* Row 2: Date range and search */}
            <div className="flex flex-wrap items-center gap-3 pt-3 border-t border-dark-700/50">
              {/* Date range */}
              <div className="flex items-center gap-2">
                <Calendar className="w-4 h-4 text-dark-400" />
                <input
                  type="date"
                  value={logFromDate}
                  onChange={(e) => setLogFromDate(e.target.value)}
                  className="input-field w-auto py-2 text-sm"
                  placeholder="Від"
                />
                <span className="text-dark-400">—</span>
                <input
                  type="date"
                  value={logToDate}
                  onChange={(e) => setLogToDate(e.target.value)}
                  className="input-field w-auto py-2 text-sm"
                  placeholder="До"
                />
              </div>

              {/* Card UID search */}
              <div className="relative flex-1 min-w-[200px]">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-dark-400" />
                <input
                  type="text"
                  placeholder="Пошук за назвою або UID..."
                  value={logCardUidFilter}
                  onChange={(e) => setLogCardUidFilter(e.target.value)}
                  className="input-field pl-10 py-2 text-sm"
                />
              </div>

              {/* Reset filters */}
              {(logActionFilter || logAllowedFilter || logCardUidFilter || logCardTypeFilter || logFromDate || logToDate) && (
                <button
                  onClick={() => {
                    setLogActionFilter('');
                    setLogAllowedFilter('');
                    setLogCardUidFilter('');
                    setLogCardTypeFilter('');
                    setLogFromDate('');
                    setLogToDate('');
                  }}
                  className="flex items-center gap-2 px-3 py-2 text-sm text-dark-300 hover:text-white hover:bg-dark-700 rounded-lg transition-colors"
                >
                  <RotateCcw className="w-4 h-4" />
                  Скинути
                </button>
              )}
            </div>
            
            <div className="mt-3 text-xs text-dark-400">
              Показано {wsLogs.length} {wsLogs.length === 1 ? 'запис' : wsLogs.length < 5 ? 'записи' : 'записів'}
              {logViewMode === 'tree' && <span className="ml-2">• {groupedLogs.length} карток</span>}
              {(logFromDate || logToDate) && (
                <span className="ml-2">
                  • Період: {logFromDate || '...'} — {logToDate || '...'}
                </span>
              )}
            </div>
          </div>

          {/* Logs Display - List or Tree Mode */}
          {wsLogs.length > 0 ? (
            <>
              {/* List View */}
              {logViewMode === 'list' && (
                <div className="space-y-2">
                  <AnimatePresence mode="popLayout">
                    {wsLogs.map((log, index) => (
                      <motion.div
                        key={log.id}
                        initial={{ opacity: 0, x: -20, height: 0 }}
                        animate={{ opacity: 1, x: 0, height: 'auto' }}
                        exit={{ opacity: 0, x: 20, height: 0 }}
                        transition={{ delay: index * 0.015 }}
                        className={`glass rounded-xl p-4 border-l-4 hover:bg-dark-700/30 transition-colors ${
                          log.allowed ? 'border-l-emerald-500' : 'border-l-rose-500'
                        }`}
                      >
                        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
                          {/* Left side: status, action, card info */}
                          <div className="flex flex-wrap items-center gap-3">
                            {/* Result badge */}
                            <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg ${
                              log.allowed ? 'bg-emerald-500/15 text-emerald-300' : 'bg-rose-500/15 text-rose-300'
                            }`}>
                              {log.allowed ? <CheckCircle className="w-3.5 h-3.5" /> : <XCircle className="w-3.5 h-3.5" />}
                              {log.allowed ? t.allowed : t.denied}
                            </span>

                            {/* Action badge with icon */}
                            <ActionBadge action={log.action} />

                            {/* Card Name + UID */}
                            <span className="bg-dark-800/50 px-2 py-0.5 rounded">
                              {log.card_name ? (
                                <>
                                  <span className="text-sm font-medium">{log.card_name}</span>
                                  <span className="font-mono text-xs text-dark-400 ml-1.5">({log.card_uid})</span>
                                </>
                              ) : (
                                <span className="font-mono text-sm">{log.card_uid}</span>
                              )}
                            </span>

                            {/* Card type */}
                            {log.card_type && <CardTypeBadge cardType={log.card_type} />}

                            {/* Status info if present */}
                            {log.status && log.status !== 'success' && log.status !== 'allowed' && (
                              <span className="text-xs text-dark-400 italic">
                                {log.status}
                              </span>
                            )}
                          </div>

                          {/* Right side: device, timestamp */}
                          <div className="flex items-center gap-4 text-sm text-dark-400">
                            {log.device_id && (
                              <span className="flex items-center gap-1.5">
                                <Cpu className="w-3.5 h-3.5" />
                                {log.device_id}
                              </span>
                            )}
                            <span className="text-xs whitespace-nowrap">{formatDate(log.created_at)}</span>
                          </div>
                        </div>
                      </motion.div>
                    ))}
                  </AnimatePresence>
                </div>
              )}

              {/* Tree View - Grouped by Card */}
              {logViewMode === 'tree' && (
                <div className="space-y-3">
                  <AnimatePresence mode="popLayout">
                    {groupedLogs.map((group, groupIndex) => {
                      const isExpanded = expandedCards.has(group.card_uid);
                      const typeConfig = cardTypeConfig[group.card_type] || cardTypeConfig.UNKNOWN;
                      const scenarios = cardTypeScenarios[group.card_type] || cardTypeScenarios.UNKNOWN;
                      
                      return (
                        <motion.div
                          key={group.card_uid}
                          initial={{ opacity: 0, y: -10 }}
                          animate={{ opacity: 1, y: 0 }}
                          exit={{ opacity: 0, y: 10 }}
                          transition={{ delay: groupIndex * 0.03 }}
                          className="glass rounded-xl overflow-hidden"
                        >
                          {/* Card Header - Clickable */}
                          <button
                            onClick={() => toggleCardExpanded(group.card_uid)}
                            className={`w-full p-4 flex items-center justify-between hover:bg-dark-700/30 transition-colors ${
                              group.hasErrors ? 'border-l-4 border-l-rose-500' : 'border-l-4 border-l-emerald-500'
                            }`}
                          >
                            <div className="flex items-center gap-4">
                              {/* Expand/Collapse icon */}
                              {isExpanded ? (
                                <ChevronDown className="w-5 h-5 text-dark-400" />
                              ) : (
                                <ChevronRight className="w-5 h-5 text-dark-400" />
                              )}

                              {/* Card type icon */}
                              <div className={`p-2 rounded-lg ${typeConfig.bgColor}`}>
                                <span className={typeConfig.color}>{typeConfig.icon}</span>
                              </div>

                              {/* Card info */}
                              <div className="text-left">
                                <div className="flex items-center gap-2">
                                  {group.card_name ? (
                                    <>
                                      <span className="font-medium">{group.card_name}</span>
                                      <span className="font-mono text-xs text-dark-400">({group.card_uid})</span>
                                    </>
                                  ) : (
                                    <span className="font-mono font-medium">{group.card_uid}</span>
                                  )}
                                  <CardTypeBadge cardType={group.card_type} />
                                </div>
                                <div className="text-xs text-dark-400 mt-0.5">
                                  Остання дія: <ActionBadge action={group.lastAction} />
                                  <span className="ml-2">{formatDate(group.lastTime)}</span>
                                </div>
                              </div>
                            </div>

                            {/* Stats */}
                            <div className="flex items-center gap-3">
                              {/* Success/Fail counts */}
                              <div className="flex items-center gap-2 text-sm">
                                <span className="flex items-center gap-1 text-emerald-400">
                                  <CheckCircle className="w-4 h-4" />
                                  {group.successCount}
                                </span>
                                {group.failCount > 0 && (
                                  <span className="flex items-center gap-1 text-rose-400">
                                    <XCircle className="w-4 h-4" />
                                    {group.failCount}
                                  </span>
                                )}
                              </div>
                              <span className="text-dark-500 text-sm">{group.logs.length} дій</span>
                            </div>
                          </button>

                          {/* Expanded Content - Operation Timeline */}
                          {isExpanded && (
                            <motion.div
                              initial={{ height: 0, opacity: 0 }}
                              animate={{ height: 'auto', opacity: 1 }}
                              exit={{ height: 0, opacity: 0 }}
                              className="border-t border-dark-700/50"
                            >
                              {/* Available scenarios for this card type */}
                              <div className="px-4 py-2 bg-dark-800/30 border-b border-dark-700/30">
                                <span className="text-xs text-dark-500">Можливі сценарії для {cardTypeLabels[group.card_type]}:</span>
                                <div className="flex flex-wrap gap-1.5 mt-1">
                                  {scenarios.map(scenario => (
                                    <span
                                      key={scenario}
                                      className={`text-xs px-2 py-0.5 rounded ${
                                        group.logs.some(l => l.action === scenario)
                                          ? 'bg-primary-500/20 text-primary-300'
                                          : 'bg-dark-700/50 text-dark-500'
                                      }`}
                                    >
                                      {actionConfig[scenario]?.label || scenario}
                                    </span>
                                  ))}
                                </div>
                              </div>

                              {/* Operation Timeline */}
                              <div className="p-4 pl-8">
                                <div className="relative">
                                  {/* Vertical line */}
                                  <div className="absolute left-2 top-0 bottom-0 w-0.5 bg-dark-700" />
                                  
                                  {/* Timeline items */}
                                  <div className="space-y-3">
                                    {group.logs.map((log) => (
                                      <div key={log.id} className="relative pl-8">
                                        {/* Timeline dot */}
                                        <div className={`absolute left-0 top-2 w-4 h-4 rounded-full border-2 ${
                                          log.allowed 
                                            ? 'bg-emerald-500/20 border-emerald-500' 
                                            : 'bg-rose-500/20 border-rose-500'
                                        }`} />
                                        
                                        {/* Log entry */}
                                        <div className={`p-3 rounded-lg bg-dark-800/50 border-l-2 ${
                                          log.allowed ? 'border-l-emerald-500/50' : 'border-l-rose-500/50'
                                        }`}>
                                          <div className="flex flex-wrap items-center gap-2 mb-1">
                                            <ActionBadge action={log.action} />
                                            <span className={`text-xs px-1.5 py-0.5 rounded ${
                                              log.allowed ? 'bg-emerald-500/10 text-emerald-400' : 'bg-rose-500/10 text-rose-400'
                                            }`}>
                                              {log.allowed ? t.allowed : t.denied}
                                            </span>
                                            {log.status && log.status !== 'success' && (
                                              <span className="text-xs text-dark-400 italic">{log.status}</span>
                                            )}
                                          </div>
                                          <div className="flex items-center gap-3 text-xs text-dark-400">
                                            {log.device_id && (
                                              <span className="flex items-center gap-1">
                                                <Cpu className="w-3 h-3" />
                                                {log.device_id}
                                              </span>
                                            )}
                                            <span>{formatDate(log.created_at)}</span>
                                          </div>
                                        </div>
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              </div>
                            </motion.div>
                          )}
                        </motion.div>
                      );
                    })}
                  </AnimatePresence>
                </div>
              )}
            </>
          ) : (
            <div className="text-center py-20">
              <FileText className="w-16 h-16 text-dark-500 mx-auto mb-4" />
              <p className="text-xl text-dark-300 mb-2">
                {wsConnected ? 'Очікування логів...' : 'Підключення...'}
              </p>
              <p className="text-dark-500">
                Логи доступу з'являться автоматично
              </p>
            </div>
          )}
        </motion.div>
      )}
    </div>
  );
}
