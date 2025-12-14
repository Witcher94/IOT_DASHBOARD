import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { motion, AnimatePresence } from 'framer-motion';
import { X, Share2, Mail, Trash2, UserCheck, Eye, Edit } from 'lucide-react';
import { devicesApi } from '../services/api';
import toast from 'react-hot-toast';

interface ShareModalProps {
  isOpen: boolean;
  onClose: () => void;
  deviceId: string;
  deviceName: string;
}

export default function ShareModal({ isOpen, onClose, deviceId, deviceName }: ShareModalProps) {
  const queryClient = useQueryClient();
  const [email, setEmail] = useState('');
  const [permission, setPermission] = useState<'view' | 'edit'>('view');

  const { data: shares, isLoading } = useQuery({
    queryKey: ['device-shares', deviceId],
    queryFn: () => devicesApi.getShares(deviceId),
    enabled: isOpen,
  });

  const shareMutation = useMutation({
    mutationFn: () => devicesApi.shareDevice(deviceId, email, permission),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['device-shares', deviceId] });
      setEmail('');
      toast.success('Device shared successfully');
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to share device');
    },
  });

  const removeMutation = useMutation({
    mutationFn: (userId: string) => devicesApi.removeShare(deviceId, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['device-shares', deviceId] });
      toast.success('Share removed');
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to remove share');
    },
  });

  const handleShare = (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim()) return;
    shareMutation.mutate();
  };

  if (!isOpen) return null;

  return (
    <AnimatePresence>
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50 flex items-center justify-center p-4"
        onClick={onClose}
      >
        <motion.div
          initial={{ scale: 0.95, opacity: 0 }}
          animate={{ scale: 1, opacity: 1 }}
          exit={{ scale: 0.95, opacity: 0 }}
          className="bg-dark-800 rounded-2xl w-full max-w-md border border-dark-700 shadow-xl"
          onClick={e => e.stopPropagation()}
        >
          {/* Header */}
          <div className="flex items-center justify-between p-4 border-b border-dark-700">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-primary-500/20 rounded-lg">
                <Share2 className="w-5 h-5 text-primary-400" />
              </div>
              <div>
                <h2 className="text-lg font-semibold">Share Device</h2>
                <p className="text-sm text-dark-400">{deviceName}</p>
              </div>
            </div>
            <button
              onClick={onClose}
              className="p-2 hover:bg-dark-700 rounded-lg transition-colors"
            >
              <X className="w-5 h-5" />
            </button>
          </div>

          {/* Share Form */}
          <form onSubmit={handleShare} className="p-4 border-b border-dark-700">
            <div className="flex gap-2">
              <div className="flex-1 relative">
                <Mail className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-dark-400" />
                <input
                  type="email"
                  value={email}
                  onChange={e => setEmail(e.target.value)}
                  placeholder="Enter email address"
                  className="w-full pl-10 pr-4 py-2.5 bg-dark-900 border border-dark-600 rounded-lg focus:outline-none focus:border-primary-500 text-sm"
                />
              </div>
              <select
                value={permission}
                onChange={e => setPermission(e.target.value as 'view' | 'edit')}
                className="px-3 py-2 bg-dark-900 border border-dark-600 rounded-lg text-sm"
              >
                <option value="view">View</option>
                <option value="edit">Edit</option>
              </select>
              <button
                type="submit"
                disabled={shareMutation.isPending || !email.trim()}
                className="px-4 py-2 bg-primary-500 hover:bg-primary-600 disabled:bg-dark-600 disabled:cursor-not-allowed rounded-lg text-sm font-medium transition-colors"
              >
                Share
              </button>
            </div>
          </form>

          {/* Shares List */}
          <div className="p-4 max-h-64 overflow-y-auto">
            <h3 className="text-sm font-medium text-dark-400 mb-3">Shared with</h3>
            {isLoading ? (
              <div className="text-center py-4 text-dark-500">Loading...</div>
            ) : shares?.length ? (
              <div className="space-y-2">
                {shares.map(share => (
                  <div
                    key={share.id}
                    className="flex items-center justify-between p-3 bg-dark-900/50 rounded-lg"
                  >
                    <div className="flex items-center gap-3">
                      <div className="p-2 bg-dark-700 rounded-full">
                        <UserCheck className="w-4 h-4 text-green-400" />
                      </div>
                      <div>
                        <p className="font-medium text-sm">{share.shared_with_name}</p>
                        <p className="text-xs text-dark-400">{share.shared_with_email}</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className={`flex items-center gap-1 px-2 py-1 rounded text-xs ${
                        share.permission === 'edit' 
                          ? 'bg-yellow-500/20 text-yellow-400'
                          : 'bg-blue-500/20 text-blue-400'
                      }`}>
                        {share.permission === 'edit' ? <Edit className="w-3 h-3" /> : <Eye className="w-3 h-3" />}
                        {share.permission}
                      </span>
                      <button
                        onClick={() => removeMutation.mutate(share.shared_with_id)}
                        disabled={removeMutation.isPending}
                        className="p-1.5 hover:bg-red-500/20 text-dark-400 hover:text-red-400 rounded transition-colors"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-center py-6 text-dark-500">
                <Share2 className="w-8 h-8 mx-auto mb-2 opacity-50" />
                <p>Not shared with anyone yet</p>
              </div>
            )}
          </div>
        </motion.div>
      </motion.div>
    </AnimatePresence>
  );
}

