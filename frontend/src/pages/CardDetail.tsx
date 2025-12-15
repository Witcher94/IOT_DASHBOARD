import { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { motion } from 'framer-motion';
import {
  ArrowLeft,
  CreditCard,
  CheckCircle,
  Clock,
  XCircle,
  Trash2,
  Pencil,
  Check,
  X,
  Copy,
  RefreshCw,
  Key,
  Shield,
  AlertTriangle,
} from 'lucide-react';
import toast from 'react-hot-toast';
import { skudApi, devicesApi } from '../services/api';
import { useTranslation } from '../contexts/settingsStore';
import type { CardStatus, Device } from '../types';

const statusColors: Record<CardStatus, string> = {
  pending: 'bg-amber-500/15 text-amber-300 border-amber-500/30',
  active: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
  disabled: 'bg-rose-500/15 text-rose-300 border-rose-500/30',
};

const statusIcons: Record<CardStatus, React.ReactNode> = {
  pending: <Clock className="w-5 h-5" />,
  active: <CheckCircle className="w-5 h-5" />,
  disabled: <XCircle className="w-5 h-5" />,
};

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleString();
}

export default function CardDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const t = useTranslation();
  const queryClient = useQueryClient();

  // Fetch card details
  const { data: card, isLoading } = useQuery({
    queryKey: ['card', id],
    queryFn: () => skudApi.getCard(id!),
    enabled: !!id,
  });

  // Fetch all SKUD devices for linking
  const { data: allDevices } = useQuery({
    queryKey: ['devices'],
    queryFn: devicesApi.getAll,
  });

  const skudDevices = allDevices?.filter((d: Device) => d.device_type === 'skud') || [];

  // Mutations
  const updateStatusMutation = useMutation({
    mutationFn: (status: CardStatus) => skudApi.updateCardStatus(id!, { status }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['card', id] });
      queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
      toast.success(t.cardStatusUpdated);
    },
    onError: () => toast.error(t.error),
  });

  const deleteMutation = useMutation({
    mutationFn: () => skudApi.deleteCard(id!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
      toast.success(t.delete);
      navigate('/skud');
    },
    onError: () => toast.error(t.error),
  });

  const linkCardMutation = useMutation({
    mutationFn: (deviceId: string) => skudApi.linkCardToDevice(id!, deviceId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['card', id] });
      queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
      toast.success(t.linkCard);
    },
    onError: () => toast.error(t.error),
  });

  const unlinkCardMutation = useMutation({
    mutationFn: (deviceId: string) => skudApi.unlinkCardFromDevice(id!, deviceId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['card', id] });
      queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
      toast.success(t.unlinkCard);
    },
    onError: () => toast.error(t.error),
  });

  const updateCardMutation = useMutation({
    mutationFn: (name: string) => skudApi.updateCard(id!, { name }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['card', id] });
      queryClient.invalidateQueries({ queryKey: ['skud-cards'] });
      toast.success(t.cardUpdated || 'Картку оновлено');
      setIsEditing(false);
    },
    onError: () => toast.error(t.error),
  });

  const regenerateTokenMutation = useMutation({
    mutationFn: () => skudApi.regenerateCardToken(id!),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['card', id] });
      setNewToken(data.token);
      toast.success('Токен перегенеровано. Старий токен дійсний ще 24 години.');
    },
    onError: () => toast.error(t.error),
  });

  const regenerateDesfireKeyMutation = useMutation({
    mutationFn: () => skudApi.regenerateDesfireKey(id!),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['card', id] });
      toast.success(`Ключ v${data.key_version} заплановано. Прикладіть картку для оновлення.`);
    },
    onError: (error: Error & { response?: { data?: { error_code?: string } } }) => {
      if (error.response?.data?.error_code === 'ALREADY_PENDING') {
        toast.error('Оновлення ключа вже заплановано. Прикладіть картку.');
      } else {
        toast.error(t.error);
      }
    },
  });

  // Editing state
  const [isEditing, setIsEditing] = useState(false);
  const [editingName, setEditingName] = useState('');
  const [newToken, setNewToken] = useState<string | null>(null);
  const [copiedToken, setCopiedToken] = useState(false);

  const copyToken = async (token: string) => {
    await navigator.clipboard.writeText(token);
    setCopiedToken(true);
    toast.success('Токен скопійовано');
    setTimeout(() => setCopiedToken(false), 2000);
  };

  const startEditing = () => {
    setEditingName(card?.name || '');
    setIsEditing(true);
  };

  const saveCardName = () => {
    updateCardMutation.mutate(editingName);
  };

  const copyUid = () => {
    if (card) {
      navigator.clipboard.writeText(card.card_uid);
      toast.success('UID скопійовано');
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="w-12 h-12 spinner" />
      </div>
    );
  }

  if (!card) {
    return (
      <div className="p-8">
        <p className="text-center text-dark-400">Card not found</p>
      </div>
    );
  }

  return (
    <div className="p-8 max-w-4xl mx-auto">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        className="mb-8"
      >
        <button
          onClick={() => navigate('/skud')}
          className="flex items-center gap-2 text-dark-400 hover:text-white transition-colors mb-4"
        >
          <ArrowLeft className="w-5 h-5" />
          {t.cards || 'Cards'}
        </button>

        <div className="flex items-start gap-4">
          <div className="p-4 rounded-2xl bg-gradient-to-br from-primary-500/20 to-accent-400/20">
            <CreditCard className="w-8 h-8 text-primary-400" />
          </div>
          <div className="flex-1">
            {/* Editable Name */}
            {isEditing ? (
              <div className="flex items-center gap-2 mb-2">
                <input
                  type="text"
                  value={editingName}
                  onChange={(e) => setEditingName(e.target.value)}
                  placeholder="Ім'я картки..."
                  className="input-field text-2xl font-bold py-1 flex-1"
                  autoFocus
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') saveCardName();
                    if (e.key === 'Escape') setIsEditing(false);
                  }}
                />
                <button
                  onClick={saveCardName}
                  disabled={updateCardMutation.isPending}
                  className="p-2 rounded-lg bg-emerald-500/20 text-emerald-300 hover:bg-emerald-500/30 transition-colors"
                >
                  <Check className="w-5 h-5" />
                </button>
                <button
                  onClick={() => setIsEditing(false)}
                  className="p-2 rounded-lg bg-dark-700 text-dark-300 hover:bg-dark-600 transition-colors"
                >
                  <X className="w-5 h-5" />
                </button>
              </div>
            ) : (
              <div className="flex items-center gap-2 mb-1 group">
                <h1 className="text-2xl font-bold">
                  {card.name || <span className="text-dark-400 italic">Без імені</span>}
                </h1>
                <button
                  onClick={startEditing}
                  className="p-1.5 rounded-lg text-dark-500 hover:text-primary-400 hover:bg-dark-700 transition-colors opacity-0 group-hover:opacity-100"
                >
                  <Pencil className="w-4 h-4" />
                </button>
              </div>
            )}
            
            {/* UID with copy button */}
            <div className="flex items-center gap-2 mb-2">
              <span className="font-mono text-sm text-dark-400">UID: {card.card_uid}</span>
              <button
                onClick={copyUid}
                className="p-1 rounded text-dark-500 hover:text-primary-400 transition-colors"
                title="Копіювати UID"
              >
                <Copy className="w-3.5 h-3.5" />
              </button>
            </div>

            {/* Card type and creation date */}
            <div className="flex items-center gap-3 text-sm text-dark-400">
              {card.card_type && (
                <span className="px-2 py-0.5 rounded-full bg-dark-700/50 text-dark-300 border border-dark-600">
                  {card.card_type.replace(/_/g, ' ')}
                </span>
              )}
              <span>{formatDate(card.created_at)}</span>
            </div>
          </div>
        </div>
      </motion.div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Status Section */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.1 }}
          className="glass rounded-xl p-6"
        >
          <h2 className="text-lg font-semibold mb-4">{t.cardStatus || 'Status'}</h2>
          
          {/* Current Status */}
          <div className="mb-6">
            <p className="text-sm text-dark-400 mb-2">{t.cardStatus || 'Current Status'}:</p>
            <span className={`inline-flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-full border ${statusColors[card.status]}`}>
              {statusIcons[card.status]}
              {card.status === 'pending' ? t.pending : card.status === 'active' ? t.active : t.disabled}
            </span>
          </div>

          {/* Status Actions */}
          <div className="space-y-3">
            <p className="text-sm text-dark-400">Change Status:</p>
            <div className="flex flex-wrap gap-3">
              {card.status !== 'active' && (
                <button
                  onClick={() => updateStatusMutation.mutate('active')}
                  disabled={updateStatusMutation.isPending}
                  className="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-lg bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/20 transition-colors border border-emerald-500/30"
                >
                  <CheckCircle className="w-4 h-4" />
                  {t.active || 'Activate'}
                </button>
              )}
              {card.status !== 'pending' && (
                <button
                  onClick={() => updateStatusMutation.mutate('pending')}
                  disabled={updateStatusMutation.isPending}
                  className="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-lg bg-amber-500/10 text-amber-300 hover:bg-amber-500/20 transition-colors border border-amber-500/30"
                >
                  <Clock className="w-4 h-4" />
                  {t.pending || 'Set Pending'}
                </button>
              )}
              {card.status !== 'disabled' && (
                <button
                  onClick={() => updateStatusMutation.mutate('disabled')}
                  disabled={updateStatusMutation.isPending}
                  className="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-lg bg-rose-500/10 text-rose-300 hover:bg-rose-500/20 transition-colors border border-rose-500/30"
                >
                  <XCircle className="w-4 h-4" />
                  {t.disabled || 'Disable'}
                </button>
              )}
            </div>
          </div>

          {/* Timestamps */}
          <div className="mt-6 pt-6 border-t border-dark-700/50 space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-dark-400">Created:</span>
              <span>{formatDate(card.created_at)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-dark-400">Updated:</span>
              <span>{formatDate(card.updated_at)}</span>
            </div>
          </div>
        </motion.div>

        {/* Token Section */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.15 }}
          className="glass rounded-xl p-6"
        >
          <div className="flex items-center gap-2 mb-4">
            <Key className="w-5 h-5 text-primary-400" />
            <h2 className="text-lg font-semibold">Токен автентифікації</h2>
          </div>
          
          <p className="text-sm text-dark-400 mb-4">
            Токен використовується для верифікації картки. При перегенерації старий токен залишається дійсним 24 години.
          </p>

          {/* Current/New Token Display */}
          {(newToken || card.token) && (
            <div className="mb-4">
              <p className="text-xs text-dark-400 mb-2">
                {newToken ? 'Новий токен:' : 'Поточний токен:'}
              </p>
              <div className="relative">
                <input
                  type="text"
                  value={newToken || card.token || ''}
                  readOnly
                  className="input-field pr-12 font-mono text-xs"
                />
                <button
                  onClick={() => copyToken(newToken || card.token || '')}
                  className="absolute right-3 top-1/2 -translate-y-1/2 p-1.5 rounded-lg hover:bg-dark-600 transition-colors"
                >
                  {copiedToken ? (
                    <Check className="w-4 h-4 text-green-400" />
                  ) : (
                    <Copy className="w-4 h-4 text-dark-400" />
                  )}
                </button>
              </div>
              {newToken && (
                <p className="text-xs text-amber-400 mt-2">
                  ⚠️ Збережіть токен! Він більше не буде показаний.
                </p>
              )}
            </div>
          )}

          {/* Regenerate Button */}
          <button
            onClick={() => {
              if (confirm('Ви впевнені? Старий токен буде дійсний ще 24 години.')) {
                regenerateTokenMutation.mutate();
              }
            }}
            disabled={regenerateTokenMutation.isPending}
            className="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-lg bg-primary-500/10 text-primary-300 hover:bg-primary-500/20 transition-colors border border-primary-500/30"
          >
            {regenerateTokenMutation.isPending ? (
              <RefreshCw className="w-4 h-4 animate-spin" />
            ) : (
              <RefreshCw className="w-4 h-4" />
            )}
            Перегенерувати токен
          </button>
        </motion.div>

        {/* DESFire Key Section - only for DESFire cards */}
        {card.card_type === 'MIFARE_DESFIRE' && (
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.17 }}
            className="glass rounded-xl p-6"
          >
            <div className="flex items-center gap-2 mb-4">
              <Shield className="w-5 h-5 text-emerald-400" />
              <h2 className="text-lg font-semibold">DESFire криптографічний ключ</h2>
            </div>
            
            <p className="text-sm text-dark-400 mb-4">
              Унікальний AES-128 ключ, записаний на картку. Перегенерація зробить старий ключ недійсним після оновлення.
            </p>

            {/* Key Version Info */}
            <div className="flex items-center gap-4 mb-4 p-3 rounded-lg bg-dark-800/50">
              <div>
                <p className="text-xs text-dark-400">Версія ключа</p>
                <p className="text-lg font-bold text-emerald-400">v{card.key_version || 0}</p>
              </div>
              
              <div className="border-l border-dark-600 pl-4">
                <p className="text-xs text-dark-400">Лічильник (ATC)</p>
                <p className="text-lg font-bold text-blue-400">{card.last_counter || 0}</p>
              </div>

              {card.pending_key_update && (
                <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-amber-500/15 border border-amber-500/30 ml-auto">
                  <AlertTriangle className="w-4 h-4 text-amber-400" />
                  <span className="text-sm text-amber-300">Очікує оновлення</span>
                </div>
              )}
            </div>

            {/* Pending Update Notice */}
            {card.pending_key_update && (
              <div className="mb-4 p-3 rounded-lg bg-amber-500/10 border border-amber-500/20">
                <p className="text-sm text-amber-300">
                  ⏳ Новий ключ заплановано. Прикладіть картку до читача для оновлення.
                </p>
              </div>
            )}

            {/* Regenerate DESFire Key Button */}
            <button
              onClick={() => {
                if (confirm('Ви впевнені? Новий ключ буде записаний на картку при наступному прикладанні.')) {
                  regenerateDesfireKeyMutation.mutate();
                }
              }}
              disabled={regenerateDesfireKeyMutation.isPending || card.pending_key_update}
              className={`flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-lg transition-colors border ${
                card.pending_key_update
                  ? 'bg-dark-700 text-dark-400 border-dark-600 cursor-not-allowed'
                  : 'bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/20 border-emerald-500/30'
              }`}
            >
              {regenerateDesfireKeyMutation.isPending ? (
                <RefreshCw className="w-4 h-4 animate-spin" />
              ) : (
                <Shield className="w-4 h-4" />
              )}
              {card.pending_key_update ? 'Оновлення заплановано' : 'Перегенерувати ключ'}
            </button>
          </motion.div>
        )}

        {/* Linked Devices Section */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2 }}
          className="glass rounded-xl p-6"
        >
          <h2 className="text-lg font-semibold mb-4">{t.linkedDevices || 'Linked Devices'}</h2>
          
          <p className="text-sm text-dark-400 mb-4">
            Картка працює тільки на пристроях, до яких вона прив'язана.
          </p>

          {skudDevices.length > 0 ? (
            <div className="space-y-2">
              {skudDevices.map((device: Device) => {
                const isLinked = card.devices?.some((d) => d.id === device.id);
                return (
                  <div
                    key={device.id}
                    className={`flex items-center justify-between p-3 rounded-lg transition-all ${
                      isLinked
                        ? 'bg-primary-500/10 border border-primary-500/30'
                        : 'bg-dark-800/50 border border-transparent'
                    }`}
                  >
                    <div className="flex items-center gap-3">
                      <span className={`w-2 h-2 rounded-full ${device.is_online ? 'bg-emerald-400' : 'bg-rose-400'}`} />
                      <span className={isLinked ? 'text-white' : 'text-dark-300'}>{device.name}</span>
                    </div>
                    <button
                      onClick={() => {
                        if (isLinked) {
                          unlinkCardMutation.mutate(device.id);
                        } else {
                          linkCardMutation.mutate(device.id);
                        }
                      }}
                      disabled={linkCardMutation.isPending || unlinkCardMutation.isPending}
                      className={`px-3 py-1 text-xs font-medium rounded-lg transition-all ${
                        isLinked
                          ? 'bg-rose-500/20 text-rose-300 hover:bg-rose-500/30'
                          : 'bg-emerald-500/20 text-emerald-300 hover:bg-emerald-500/30'
                      }`}
                    >
                      {isLinked ? t.unlinkCard || 'Unlink' : t.linkCard || 'Link'}
                    </button>
                  </div>
                );
              })}
            </div>
          ) : (
            <p className="text-dark-500 text-center py-8">
              {t.noSkudDevices || 'No SKUD devices found'}
            </p>
          )}
        </motion.div>
      </div>

      {/* Danger Zone */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.3 }}
        className="mt-6 glass rounded-xl p-6 border border-rose-500/20"
      >
        <h2 className="text-lg font-semibold mb-4 text-rose-400">Danger Zone</h2>
        <p className="text-sm text-dark-400 mb-4">
          Видалення картки є незворотною дією. Картка буде видалена з усіх пристроїв.
        </p>
        <button
          onClick={() => {
            if (confirm('Ви впевнені що хочете видалити цю картку?')) {
              deleteMutation.mutate();
            }
          }}
          disabled={deleteMutation.isPending}
          className="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-lg bg-rose-500/10 text-rose-300 hover:bg-rose-500/20 transition-colors border border-rose-500/30"
        >
          <Trash2 className="w-4 h-4" />
          {t.delete || 'Delete Card'}
        </button>
      </motion.div>
    </div>
  );
}

