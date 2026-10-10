'use client';

/**
 * 玩家 HUD —— 显式从 useGameStore 读，不持有私有 state。
 * 坐标、tile 跟随 WorldMap 的 setPosition 实时更新。
 */

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useGameStore } from '@/store/game';
import { api, type InventoryItem, type TransactionListResponse } from '@/lib/api';
import { CreatorMarket } from './CreatorMarket';
import { CreatorRevenue } from './CreatorRevenue';
import { CreatorStudio } from './CreatorStudio';

function logout() {
  window.localStorage.removeItem('aicity_token');
  window.localStorage.removeItem('aicity_username');
  api.setToken('');
  const guestId = `guest_${Date.now().toString(36)}`;
  window.localStorage.setItem('aicity_player_id', guestId);
  useGameStore.getState().setGuestPlayer(guestId);
}

const TX_LABELS: Record<string, string> = {
  npc_purchase: 'NPC购买',
  player_transfer: '转账',
  cross_city_out: '跨城',
};

export function PlayerHUD() {
  const playerId = useGameStore((s) => s.playerId);
  const displayName = useGameStore((s) => s.displayName);
  const sessionStatus = useGameStore((s) => s.sessionStatus);
  const isGuest = useGameStore((s) => s.isGuest);
  const wallet = useGameStore((s) => s.wallet);
  const [showTransactions, setShowTransactions] = useState(false);
  const [transactions, setTransactions] = useState<TransactionListResponse | null>(null);
  const [showInventory, setShowInventory] = useState(false);
  const [inventory, setInventory] = useState<InventoryItem[] | null>(null);
  const inventoryVersion = useGameStore((s) => s.inventoryVersion);
  const [showTransfer, setShowTransfer] = useState(false);
  const [showMarket, setShowMarket] = useState(false);
  const [showRevenue, setShowRevenue] = useState(false);
  const [showStudio, setShowStudio] = useState(false);
  const [marketVersion, setMarketVersion] = useState(0);
  const [toUserId, setToUserId] = useState('');
  const [transferAmount, setTransferAmount] = useState('1');
  const [transferMemo, setTransferMemo] = useState('');
  const [transferError, setTransferError] = useState<string | null>(null);
  const [transferSuccess, setTransferSuccess] = useState<string | null>(null);
  const [transferring, setTransferring] = useState(false);
  const position = useGameStore((s) => s.position);
  const currentTileId = useGameStore((s) => s.currentTileId);

  useEffect(() => {
    if (sessionStatus !== 'authenticated') {
      useGameStore.getState().setWallet(null);
      return;
    }
    let cancelled = false;
    api
      .getWallet()
      .then((response) => {
        if (cancelled) return;
        useGameStore.getState().setWallet({
          gold: response.gold_balance,
          token: response.token_balance,
        });
      })
      .catch(() => {
        if (cancelled) return;
        useGameStore.getState().setWallet(null);
      });
    return () => {
      cancelled = true;
    };
  }, [sessionStatus, playerId]);

  useEffect(() => {
    if (!showTransactions || sessionStatus !== 'authenticated') return;
    let cancelled = false;
    api
      .getTransactions()
      .then((response) => {
        if (cancelled) return;
        setTransactions(response);
      })
      .catch(() => {
        if (cancelled) return;
        setTransactions(null);
      });
    return () => {
      cancelled = true;
    };
  }, [sessionStatus, showTransactions]);
  useEffect(() => {
    if (!showInventory || sessionStatus !== 'authenticated') return;
    let cancelled = false;
    api
      .getInventory()
      .then((items) => {
        if (cancelled) return;
        setInventory(items);
      })
      .catch(() => {
        if (cancelled) return;
        setInventory(null);
      });
    return () => {
      cancelled = true;
    };
  }, [sessionStatus, showInventory, inventoryVersion]);
  const identityLabel =
    sessionStatus === 'restoring'
      ? '恢复中...'
      : isGuest
        ? '游客'
        : displayName || '已登录';

  async function submitTransfer() {
    if (sessionStatus !== 'authenticated') return;
    const amount = Number(transferAmount);
    if (!Number.isInteger(amount) || amount <= 0) {
      setTransferError('转账数量必须是正整数');
      return;
    }
    setTransferring(true);
    setTransferError(null);
    setTransferSuccess(null);
    try {
      const response = await api.transfer({
        toUserId,
        amount,
        idempotencyKey: `city-ui-transfer-${Date.now().toString(36)}`,
        memo: transferMemo || undefined,
      });
      useGameStore.getState().setWallet({
        gold: response.gold_balance,
        token: response.token_balance,
      });
      setTransferSuccess('转账成功');
      setToUserId('');
      setTransferAmount('1');
      setTransferMemo('');
    } catch (error) {
      setTransferError(error instanceof Error ? error.message : '转账失败');
    } finally {
      setTransferring(false);
    }
  }

  return (
    <div className="absolute top-4 left-4 bg-gray-800/80 backdrop-blur p-4 rounded-lg shadow-lg">
      <div className="text-sm">
        <div className="font-bold mb-1">{identityLabel}</div>
        {!isGuest && playerId && <div className="text-xs text-gray-400">ID: {playerId}</div>}
        <div className="text-gray-400">
          坐标: {position.x.toFixed(1)}, {position.y.toFixed(1)}
        </div>
        <div className="text-gray-500">tile: {currentTileId}</div>
        <div className="text-yellow-400">
          {isGuest
            ? '余额: 登录后显示'
            : wallet
              ? `Gold: ${wallet.gold} · Token: ${wallet.token}`
              : '余额: 加载失败'}
        </div>
        <div className="mt-2 flex items-center gap-3">
          {isGuest ? (
            <Link href="/login" className="text-brand-400 hover:text-brand-300">
              登录
            </Link>
          ) : (
            <button
              type="button"
              onClick={logout}
              className="text-gray-400 hover:text-gray-200"
            >
              退出
            </button>
          )}
          {sessionStatus === 'authenticated' && (
            <>
              <button
                type="button"
                onClick={() => setShowInventory((value) => !value)}
                className="text-gray-400 hover:text-gray-200"
              >
                {showInventory ? '隐藏背包' : '背包'}
              </button>
              <button
                type="button"
                onClick={() => setShowTransfer((value) => !value)}
                className="text-gray-400 hover:text-gray-200"
              >
                {showTransfer ? '隐藏转账' : '转账'}
              </button>
              <button
                type="button"
                onClick={() => setShowMarket((value) => !value)}
                className="text-gray-400 hover:text-gray-200"
              >
                {showMarket ? '隐藏市场' : '市场'}
              </button>
              <button
                type="button"
                onClick={() => setShowRevenue((value) => !value)}
                className="text-gray-400 hover:text-gray-200"
              >
                {showRevenue ? '隐藏收益' : '收益'}
              </button>
              <button
                type="button"
                onClick={() => setShowStudio((value) => !value)}
                className="text-gray-400 hover:text-gray-200"
              >
                {showStudio ? '隐藏发布' : '发布'}
              </button>
              <button
                type="button"
                onClick={() => setShowTransactions((value) => !value)}
                className="text-gray-400 hover:text-gray-200"
              >
                {showTransactions ? '隐藏交易' : '交易'}
              </button>
            </>
          )}
        </div>

        {showInventory && (
          <div className="mt-2 max-h-32 overflow-y-auto border-t border-slate-700 pt-2">
            {inventory == null ? (
              <div className="text-xs text-gray-400">背包加载失败</div>
            ) : inventory.length === 0 ? (
              <div className="text-xs text-gray-400">暂无物品</div>
            ) : (
              <div className="space-y-1">
                {inventory.map((item) => (
                  <div
                    key={item.product_id}
                    className="flex items-center justify-between gap-2 text-xs"
                  >
                    <span className="text-gray-300">{item.name}</span>
                    <span className="text-amber-400">x{item.quantity}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {showTransfer && (
          <div className="mt-2 space-y-2 border-t border-slate-700 pt-2">
            <input
              type="text"
              value={toUserId}
              onChange={(event) => setToUserId(event.target.value)}
              placeholder="目标玩家 UUID"
              className="w-full rounded bg-slate-900 px-2 py-1 text-xs text-slate-100"
            />
            <input
              type="number"
              min={1}
              value={transferAmount}
              onChange={(event) => setTransferAmount(event.target.value)}
              placeholder="Gold 数量"
              className="w-full rounded bg-slate-900 px-2 py-1 text-xs text-slate-100"
            />
            <input
              type="text"
              value={transferMemo}
              onChange={(event) => setTransferMemo(event.target.value)}
              placeholder="备注（可选）"
              className="w-full rounded bg-slate-900 px-2 py-1 text-xs text-slate-100"
            />
            <button
              type="button"
              disabled={transferring || !toUserId}
              onClick={() => void submitTransfer()}
              className="w-full rounded bg-brand-500 px-3 py-1 text-xs font-semibold text-white hover:bg-brand-700 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {transferring ? '转账中' : '转账 Gold'}
            </button>
            {transferError && <div className="text-xs text-red-400">{transferError}</div>}
            {transferSuccess && <div className="text-xs text-green-400">{transferSuccess}</div>}
          </div>
        )}

        {showMarket && <CreatorMarket key={marketVersion} />}

        {showStudio && (
          <CreatorStudio
            onPublished={() => {
              setMarketVersion((version) => version + 1);
              setShowMarket(true);
            }}
          />
        )}

        {showRevenue && <CreatorRevenue />}

        {showTransactions && (
          <div className="mt-2 max-h-44 overflow-y-auto border-t border-slate-700 pt-2">
            {transactions == null ? (
              <div className="text-xs text-gray-400">交易加载失败</div>
            ) : transactions.transactions.length === 0 ? (
              <div className="text-xs text-gray-400">暂无交易</div>
            ) : (
              <div className="space-y-1">
                {transactions.transactions.map((tx) => (
                  <div
                    key={tx.id}
                    className="flex items-center justify-between gap-2 text-xs"
                  >
                    <span className="text-gray-300">
                      {TX_LABELS[tx.tx_type] ?? tx.tx_type}
                    </span>
                    <span
                      className={
                        tx.amount < 0 ? 'text-red-400' : 'text-green-400'
                      }
                    >
                      {tx.amount > 0 ? '+' : ''}
                      {tx.amount} {tx.currency.toUpperCase()}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
