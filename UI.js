import React, { useEffect, useMemo, useState } from "react";

// ✅ 可在 Next.js / Tailwind 環境直接使用的 React 視覺樣機（不依賴第三方 UI 套件）
// - 包含：拍賣列表頁、拍賣詳情（Running / Scheduled / Ended 狀態）、出價面板、出價紀錄、價格分布圖（SVG）
// - 右上角可切換「畫面模式」與「模擬手機寬度」
// - 反狙擊（Soft-close）行為：若剩餘 <= 180 秒，出價會自動將結束時間 +60 秒，並顯示徽章動畫
// - 所有數值為假資料，方便設計/前端對齊視覺與互動

// ---------- 小工具 ----------
const twd = (n: number) => new Intl.NumberFormat("zh-TW", { style: "currency", currency: "TWD", maximumFractionDigits: 0 }).format(n);

function classNames(...xs: (string | false | null | undefined)[]) {
  return xs.filter(Boolean).join(" ");
}

function maskUser(id: string) {
  if (id.length <= 2) return id + "*";
  return id[0] + "***" + id[id.length - 1];
}

// 倒數計時與緊急色彩
function useCountdown(target: number) {
  const [now, setNow] = useState(Date.now());
  useEffect(() => { const t = setInterval(() => setNow(Date.now()), 1000); return () => clearInterval(t); }, []);
  const remaining = Math.max(0, Math.floor((target - now) / 1000));
  let tone: "safe" | "warn" | "danger" = "safe";
  if (remaining <= 60) tone = "danger"; else if (remaining <= 300) tone = "warn";
  return { remaining, tone };
}

function RemainingText({ remaining }: { remaining: number }) {
  const m = Math.floor(remaining / 60);
  const s = remaining % 60;
  const h = Math.floor(m / 60);
  const mm = m % 60;
  if (h > 0) return <span>{h} 小時 {mm} 分 {s} 秒</span>;
  if (m > 0) return <span>{m} 分 {s} 秒</span>;
  return <span>{s} 秒</span>;
}

// ---------- 假資料 ----------
const MOCK_LIST = Array.from({ length: 8 }).map((_, i) => ({
  id: 9000 + i,
  title: `企業設備拍賣 #${9000 + i}`,
  seller: i % 2 ? "星河工業" : "大樹科技",
  img: i % 2 ? "linear-gradient(135deg,#dbeafe,#eff6ff)" : "linear-gradient(135deg,#fef9c3,#ffedd5)",
  currentPrice: 500000 + i * 25000,
  reserveMet: i % 3 !== 0,
  endAt: Date.now() + (i + 1) * 1000 * 60 * 12, // 12,24,36… 分後結束
  bids: 20 + i * 3,
}));

const MOCK_BIDS = () => {
  const base = Date.now();
  return Array.from({ length: 14 }).map((_, i) => ({
    id: 55000 + i,
    user: i % 2 ? "benny09" : "alice77",
    amount: 600000 + i * 10000,
    at: base - (14 - i) * 1000 * 32,
    proxy: i % 5 === 0,
  }));
};

// 價格分布（直方圖）假資料
const MOCK_BUCKETS = [
  { min: 580000, max: 590000, count: 2 },
  { min: 590000, max: 600000, count: 4 },
  { min: 600000, max: 610000, count: 6 },
  { min: 610000, max: 620000, count: 8 },
  { min: 620000, max: 630000, count: 11 },
  { min: 630000, max: 640000, count: 7 },
  { min: 640000, max: 650000, count: 3 },
];

// ---------- 圖表：直方圖（SVG） ----------
function Histogram({ buckets, p50, p90, p99 }: { buckets: { min: number; max: number; count: number }[]; p50: number; p90: number; p99: number }) {
  const width = 560, height = 180, pad = 24;
  const maxCount = Math.max(...buckets.map(b => b.count));
  const minX = buckets[0].min, maxX = buckets[buckets.length - 1].max;
  const x = (v: number) => pad + ((v - minX) / (maxX - minX)) * (width - pad * 2);
  const y = (c: number) => height - pad - (c / maxCount) * (height - pad * 2);

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-[200px]">
      <rect x={0} y={0} width={width} height={height} rx={12} className="fill-white" />
      {/* 軸線 */}
      <line x1={pad} y1={height - pad} x2={width - pad} y2={height - pad} className="stroke-gray-200" />
      {/* 桶狀 */}
      {buckets.map((b, i) => {
        const barW = (x(b.max) - x(b.min)) * 0.8;
        const cx = (x(b.min) + x(b.max)) / 2 - barW / 2;
        const top = y(b.count);
        return <rect key={i} x={cx} y={top} width={barW} height={(height - pad) - top} className="fill-blue-500/70 hover:fill-blue-600 transition-colors" rx={6} />;
      })}
      {/* 分位線 */}
      {[{ v: p50, c: "stroke-emerald-500" }, { v: p90, c: "stroke-amber-500" }, { v: p99, c: "stroke-red-500" }].map((l, i) => (
        <g key={i}>
          <line x1={x(l.v)} x2={x(l.v)} y1={pad} y2={height - pad} className={classNames(l.c, "stroke-[2]")} />
        </g>
      ))}
    </svg>
  );
}

// ---------- 拍賣卡片（列表） ----------
function AuctionCard({ item, onOpen }: { item: any; onOpen: (id: number) => void }) {
  const { remaining, tone } = useCountdown(item.endAt);
  return (
    <div className="rounded-2xl border border-gray-200 overflow-hidden bg-white hover:shadow-md transition-shadow">
      <div className="h-40 w-full" style={{ background: item.img }} />
      <div className="p-4 space-y-2">
        <div className="text-sm text-gray-500">{item.seller}</div>
        <h3 className="font-semibold line-clamp-1">{item.title}</h3>
        <div className="flex items-center justify-between text-sm">
          <div>
            <div className="text-gray-500">目前出價</div>
            <div className="font-bold text-lg">{twd(item.currentPrice)}</div>
          </div>
          <div className="text-right">
            <div className="text-gray-500">剩餘</div>
            <div className={classNames("font-semibold", tone === "danger" && "text-red-600", tone === "warn" && "text-amber-600", tone === "safe" && "text-gray-700")}> <RemainingText remaining={remaining} /> </div>
          </div>
        </div>
        <div className="flex items-center justify-between text-xs text-gray-500">
          <span>出價 {item.bids} 次</span>
          <span className={classNames("px-2 py-0.5 rounded-full border", item.reserveMet ? "border-emerald-200 text-emerald-700" : "border-gray-200")}>{item.reserveMet ? "已達保留價" : "未達保留價"}</span>
        </div>
        <button onClick={() => onOpen(item.id)} className="mt-2 w-full rounded-xl bg-blue-600 text-white py-2 font-medium hover:bg-blue-700">查看詳情</button>
      </div>
    </div>
  );
}

// ---------- 出價紀錄 ----------
function BidHistory({ bids, anonymize }: { bids: any[]; anonymize: boolean }) {
  return (
    <div className="divide-y divide-gray-100">
      {bids.map((b) => (
        <div key={b.id} className="flex items-center justify-between py-2 text-sm">
          <div className="text-gray-500 w-40">{new Date(b.at).toLocaleTimeString()}</div>
          <div className="flex-1">
            <span className="text-gray-700">{anonymize ? maskUser(b.user) : b.user}</span>
            {b.proxy && <span className="ml-2 text-xs text-blue-600 rounded px-2 py-0.5 bg-blue-50">代理出價</span>}
          </div>
          <div className="font-semibold">{twd(b.amount)}</div>
        </div>
      ))}
    </div>
  );
}

// ---------- 右側：即時出價面板 ----------
function LiveBidPanel({
  status,
  currentPrice,
  minIncrement,
  endAt,
  reserveMet,
  buyNow,
  anonymize,
  onPlaceBid,
  softWindow = 180,
  lastExtendedAt,
}: {
  status: "RUNNING" | "SCHEDULED" | "ENDED";
  currentPrice: number;
  minIncrement: number;
  endAt: number;
  reserveMet: boolean;
  buyNow?: number | null;
  anonymize: boolean;
  onPlaceBid: (amount: number) => void;
  softWindow?: number; // 秒
  lastExtendedAt?: number | null;
}) {
  const { remaining, tone } = useCountdown(endAt);
  const [amount, setAmount] = useState(currentPrice + minIncrement);

  useEffect(() => { setAmount(currentPrice + minIncrement); }, [currentPrice, minIncrement]);

  const disabled = status !== "RUNNING";

  return (
    <div className="sticky top-4 rounded-2xl border border-gray-200 p-5 bg-white shadow-sm space-y-4">
      <div className="text-sm text-gray-500">目前出價</div>
      <div className="text-3xl font-bold tracking-tight">{twd(currentPrice)}</div>

      <div className="flex items-center gap-2">
        <div className="text-sm text-gray-500">結束倒數</div>
        <div className={classNames("text-sm font-semibold px-2 py-1 rounded-md", tone === "danger" && "bg-red-50 text-red-600", tone === "warn" && "bg-amber-50 text-amber-700", tone === "safe" && "bg-gray-50 text-gray-700")}> <RemainingText remaining={remaining} /> </div>
        {lastExtendedAt && Date.now() - lastExtendedAt < 5000 && (
          <div className="ml-auto text-xs bg-emerald-50 text-emerald-700 px-2 py-1 rounded-lg animate-pulse">已延長 +60s</div>
        )}
      </div>

      <div className="grid grid-cols-3 gap-2 text-sm">
        <div className="rounded-xl bg-gray-50 p-3">
          <div className="text-gray-500">最小加價</div>
          <div className="font-semibold">{twd(minIncrement)}</div>
        </div>
        <div className="rounded-xl bg-gray-50 p-3">
          <div className="text-gray-500">保留價</div>
          <div className={classNames("font-semibold", reserveMet ? "text-emerald-700" : "text-gray-700")}>{reserveMet ? "已達" : "未達"}</div>
        </div>
        <div className="rounded-xl bg-gray-50 p-3">
          <div className="text-gray-500">匿名出價</div>
          <div className="font-semibold">{anonymize ? "是" : "否"}</div>
        </div>
      </div>

      <div className="space-y-2">
        <label className="text-sm text-gray-700">出價金額</label>
        <div className="flex gap-2">
          <input type="number" className="flex-1 rounded-xl border px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500" value={amount} onChange={e => setAmount(Number(e.target.value))} />
          <button className="rounded-xl border px-3 py-2 text-sm" onClick={() => setAmount(currentPrice + minIncrement)}>填入最低</button>
        </div>
        <div className="flex gap-2 text-xs">
          {[1,2,5].map(x => (
            <button key={x} className="px-3 py-1.5 rounded-lg bg-gray-100 hover:bg-gray-200" onClick={() => setAmount((v) => v + minIncrement * x)}>+{x} 檔</button>
          ))}
        </div>
      </div>

      <button disabled={disabled} onClick={() => onPlaceBid(amount)} className={classNames("w-full rounded-xl py-3 font-semibold", disabled ? "bg-gray-200 text-gray-500 cursor-not-allowed" : "bg-blue-600 text-white hover:bg-blue-700")}>{status === "RUNNING" ? "出價" : status === "SCHEDULED" ? "尚未開始" : "已結束"}</button>

      {!!buyNow && (
        <button disabled={disabled} className={classNames("w-full rounded-xl py-2 font-semibold border", disabled ? "border-gray-200 text-gray-400" : "border-blue-200 text-blue-700 hover:bg-blue-50")}>直購 {twd(buyNow)}</button>
      )}

      <div className="text-xs text-gray-500">出價即表示同意拍賣規則，並受反狙擊條款影響（臨近結束將自動延長）。</div>
    </div>
  );
}

// ---------- 拍賣詳情頁 ----------
function AuctionDetail({ variant }: { variant: "RUNNING" | "SCHEDULED" | "ENDED" }) {
  const [currentPrice, setCurrentPrice] = useState(630000);
  const minIncrement = 10000;
  const [reserveMet, setReserveMet] = useState(true);
  const [endAt, setEndAt] = useState(Date.now() + 1000 * 60 * 2 + 1000 * 20); // 預設 2m20s
  const [bids, setBids] = useState(MOCK_BIDS());
  const [anonymize, setAnonymize] = useState(false);
  const [lastExtendedAt, setLastExtendedAt] = useState<number | null>(null);

  // 模擬下單（含反狙擊）
  function placeBid(amount: number) {
    const newBid = { id: Math.floor(Math.random() * 100000), user: "you", amount, at: Date.now(), proxy: false };
    setBids((prev) => [newBid, ...prev].slice(0, 50));
    if (amount >= currentPrice + minIncrement) {
      setCurrentPrice(amount);
    }
    const remaining = Math.max(0, Math.floor((endAt - Date.now()) / 1000));
    if (remaining <= 180) { // 3 分鐘內 → 延長 60s
      setEndAt((t) => t + 60_000);
      setLastExtendedAt(Date.now());
    }
  }

  // 直方圖分位數（示意）
  const p50 = 620000, p90 = 640000, p99 = 648000;

  return (
    <div className="grid grid-cols-1 xl:grid-cols-12 gap-8">
      {/* 左側主內容 */}
      <div className="xl:col-span-8 space-y-6">
        <div className="rounded-3xl border border-gray-200 bg-white overflow-hidden">
          <div className="grid grid-cols-3 gap-1 h-80">
            <div className="col-span-2 h-full" style={{ background: "linear-gradient(135deg,#e9d5ff,#eef2ff)" }} />
            <div className="flex flex-col gap-1">
              {Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="flex-1" style={{ background: i % 2 ? "linear-gradient(135deg,#fee2e2,#fff7ed)" : "linear-gradient(135deg,#dcfce7,#eff6ff)" }} />
              ))}
            </div>
          </div>
          <div className="p-6">
            <div className="text-sm text-gray-500">星河工業</div>
            <h1 className="text-2xl font-bold tracking-tight">高精密 CNC 加工中心 X500（保固移轉）</h1>
            <p className="mt-2 text-gray-600">設備良好，含 8 成新刀庫與校正紀錄。限台灣本島取貨，買方負擔運費與吊車。</p>
          </div>
        </div>

        {/* Tabs 區域：出價紀錄 / 價格分布 / 規則 */}
        <div className="rounded-3xl border border-gray-200 bg-white">
          <div className="flex gap-6 px-6 pt-4 text-sm">
            <a className="font-medium border-b-2 border-transparent hover:border-gray-300 cursor-default">出價紀錄</a>
            <a className="font-medium border-b-2 border-transparent hover:border-gray-300 cursor-default">價格分布</a>
            <a className="font-medium border-b-2 border-transparent hover:border-gray-300 cursor-default">規則</a>
            <div className="ml-auto flex items-center gap-3 text-xs text-gray-500">
              <label className="flex items-center gap-2 cursor-pointer select-none"><input type="checkbox" checked={anonymize} onChange={e => setAnonymize(e.target.checked)} /> 匿名顯示</label>
            </div>
          </div>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 p-6">
            <div>
              <BidHistory bids={bids} anonymize={anonymize} />
            </div>
            <div>
              <div className="flex items-center justify-between mb-2">
                <div className="text-sm text-gray-500">價格分布（每 5 分鐘更新）</div>
                <div className="text-xs text-gray-400">上次更新：{new Date(Date.now() - 2 * 60 * 1000).toLocaleTimeString()}</div>
              </div>
              <Histogram buckets={MOCK_BUCKETS} p50={p50} p90={p90} p99={p99} />
              <div className="mt-2 text-xs text-gray-500">分位數：p50 {twd(p50)}｜p90 {twd(p90)}｜p99 {twd(p99)}</div>
            </div>
          </div>
          <div className="px-6 pb-6 text-sm text-gray-600">
            <ul className="list-disc pl-5 space-y-1">
              <li>英式（明標）加價競標，最小加價檔 {twd(10000)}。</li>
              <li>反狙擊：結束前 3 分鐘內有出價，結束時間自動延長 1 分鐘。</li>
              <li>賣家設定保留價：未達則可能流標或協商。</li>
            </ul>
          </div>
        </div>
      </div>

      {/* 右側出價面板 */}
      <div className="xl:col-span-4">
        <LiveBidPanel
          status={variant}
          currentPrice={currentPrice}
          minIncrement={minIncrement}
          endAt={endAt}
          reserveMet={reserveMet}
          buyNow={variant === "RUNNING" ? 800000 : null}
          anonymize={anonymize}
          onPlaceBid={placeBid}
          lastExtendedAt={lastExtendedAt}
        />
        {/* 狀態說明 */}
        {variant !== "RUNNING" && (
          <div className="mt-4 rounded-2xl border border-gray-200 bg-white p-4 text-sm text-gray-600">
            {variant === "SCHEDULED" && <p>此拍賣尚未開始。請於開始時間回來或設定提醒。</p>}
            {variant === "ENDED" && <p>拍賣已結束。若已達保留價，系統已通知得標者與賣家。</p>}
          </div>
        )}
      </div>
    </div>
  );
}

// ---------- 拍賣列表頁 ----------
function AuctionList({ onOpen }: { onOpen: (id: number) => void }) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
      {MOCK_LIST.map(item => <AuctionCard key={item.id} item={item} onOpen={onOpen} />)}
    </div>
  );
}

// ---------- 頁面容器（頂部導覽 + 畫面切換） ----------
export default function AuctionUIScreen() {
  const [mode, setMode] = useState<"list" | "detail-running" | "detail-scheduled" | "detail-ended">("detail-running");
  const [mobile, setMobile] = useState(false);

  return (
    <div className="min-h-screen bg-gradient-to-b from-gray-50 to-white text-gray-900">
      {/* 頂部導覽 */}
      <header className="sticky top-0 z-10 backdrop-blur bg-white/70 border-b border-gray-100">
        <div className="max-w-[1100px] mx-auto px-6 py-3 flex items-center gap-4">
          <div className="font-black tracking-tight text-xl">AuctiX</div>
          <nav className="hidden md:flex items-center gap-4 text-sm text-gray-600">
            <a className="hover:text-gray-900" href="#">瀏覽拍賣</a>
            <a className="hover:text-gray-900" href="#">我的競標</a>
            <a className="hover:text-gray-900" href="#">賣家後台</a>
          </nav>
          <div className="ml-auto flex items-center gap-3 text-sm">
            <div className="hidden sm:flex items-center gap-2 border rounded-xl px-2 py-1 bg-white">
              <span className="text-gray-500">畫面模式</span>
              <select className="bg-transparent outline-none" value={mode} onChange={e => setMode(e.target.value as any)}>
                <option value="list">列表頁</option>
                <option value="detail-running">詳情：進行中</option>
                <option value="detail-scheduled">詳情：未開始</option>
                <option value="detail-ended">詳情：已結束</option>
              </select>
            </div>
            <label className="flex items-center gap-2 cursor-pointer select-none border rounded-xl px-2 py-1 bg-white">
              <input type="checkbox" checked={mobile} onChange={e => setMobile(e.target.checked)} /> 模擬手機寬度
            </label>
            <button className="rounded-xl bg-gray-900 text-white px-3 py-1.5">登入</button>
          </div>
        </div>
      </header>

      {/* 內容 */}
      <main className="max-w-[1100px] mx-auto px-6 py-8">
        <div className={classNames("mx-auto", mobile ? "max-w-[420px]" : "")}>
          {mode === "list" && <AuctionList onOpen={() => setMode("detail-running")} />}
          {mode === "detail-running" && <AuctionDetail variant="RUNNING" />}
          {mode === "detail-scheduled" && <AuctionDetail variant="SCHEDULED" />}
          {mode === "detail-ended" && <AuctionDetail variant="ENDED" />}
        </div>
      </main>

      {/* 頁尾 */}
      <footer className="border-t border-gray-100 py-8 text-sm text-gray-500">
        <div className="max-w-[1100px] mx-auto px-6 flex items-center justify-between">
          <div>© 2025 AuctiX Co.</div>
          <div className="flex gap-4">
            <a href="#">服務條款</a>
            <a href="#">隱私權政策</a>
          </div>
        </div>
      </footer>
    </div>
  );
}
