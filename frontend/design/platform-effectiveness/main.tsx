import { useRef, useState, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { createPortal } from "react-dom";
import { AppShell } from "../../src/app/AppShell";
import { Button } from "../../src/shared/ui/Button";
import "../../src/styles/index.css";
import "./style.css";

type Row = {
  id: string;
  app: string;
  day: number;
  actor: string;
  minutes: number;
  lead: number | null;
  incident: boolean;
  recovery: number | null;
  rework: boolean;
  support: boolean | null;
};
const rows: Row[] = Array.from({ length: 48 }, (_, i) => ({
  id: `DEP-${1048 - i}`,
  app: i % 3 === 0 ? "billing-api" : "checkout-api",
  day: 23 - (i % 30),
  actor: ["Minh Anh", "Hoàng Nam", "Thu Hà", "Đức Huy"][i % 4]!,
  minutes: 6 + (i % 13),
  lead: i % 9 === 0 ? null : 80 + ((i * 37) % 320),
  incident: i % 11 === 0,
  recovery: i % 11 === 0 ? (i === 0 ? null : 20 + i) : null,
  rework: i % 13 === 0,
  support: i % 7 === 0 ? null : i % 5 === 0,
}));
const date = (day: number) =>
  new Date(Date.UTC(2026, 9, day)).toISOString().slice(0, 10);
const median = (v: number[]) => {
  const a = [...v].sort((x, y) => x - y);
  return a.length
    ? (a[Math.floor((a.length - 1) / 2)]! + a[Math.floor(a.length / 2)]!) / 2
    : 0;
};
const fmt = (n: number) =>
  new Intl.NumberFormat("vi-VN", { maximumFractionDigits: 1 }).format(n);
const duration = (n: number) =>
  n >= 60 ? `${fmt(n / 60)} giờ` : `${fmt(n)} phút`;
const tabs = ["Tổng quan", "Tự phục vụ", "Dữ liệu"];
function App() {
  const [tab, setTab] = useState("Tổng quan"),
    [app, setApp] = useState("checkout-api"),
    [days, setDays] = useState(30),
    [empty, setEmpty] = useState(false),
    [detail, setDetail] = useState<string | null>(null),
    [baseline, setBaseline] = useState<{
      minutes: number;
      source: string;
    } | null>(null),
    [notice, setNotice] = useState("");
  const [navSlot] = useState(() => document.createElement("div"));
  const shell = useRef<HTMLDivElement>(null);
  useEffect(() => {
    navSlot.className = "eff-nav-slot";
    shell.current?.querySelector(".nav")?.appendChild(navSlot);
    return () => navSlot.remove();
  }, [navSlot]);
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (detail) dialog.current?.showModal();
    else dialog.current?.close();
  }, [detail]);
  const data = empty
    ? []
    : rows.filter((r) => r.app === app && r.day > 23 - days);
  const leads = data.flatMap((r) => (r.lead === null ? [] : [r.lead]));
  const incidents = data.filter((r) => r.incident),
    recoveries = incidents.flatMap((r) =>
      r.recovery === null ? [] : [r.recovery],
    );
  const answered = data.filter((r) => r.support !== null),
    self = answered.filter((r) => r.support === false).length;
  const users = new Set(data.map((r) => r.actor)).size;
  const current = median(data.map((r) => r.minutes));
  const metrics = [
    {
      name: "Tần suất triển khai",
      en: "Deployment frequency",
      value: data.length ? `${fmt(data.length / days)} lần/ngày` : "—",
      note: `${data.length} deployment production / ${days} ngày`,
      formula:
        "Số deployment đã lên production / số ngày trong kỳ. Các bản ghi mẫu đều đã lên production; sự cố sau deploy được ghi riêng.",
    },
    {
      name: "Commit đến production",
      en: "Change lead time",
      value: leads.length ? duration(median(leads)) : "—",
      note: `Có liên kết commit: ${leads.length}/${data.length}`,
      formula:
        "Trung vị thời gian từ commit đến deployment production thành công. Không dùng thời gian chạy pipeline thay thế. Chỉ tính bản ghi có commit.",
    },
    {
      name: "Thay đổi gây lỗi",
      en: "Change fail rate",
      value: data.length
        ? `${fmt((incidents.length / data.length) * 100)}%`
        : "—",
      note: `${incidents.length}/${data.length} deployment cần can thiệp`,
      formula:
        "Deployment production gây sự cố cần can thiệp ngay / tổng deployment production. Không đồng nhất với trạng thái FAILED của pipeline.",
    },
    {
      name: "Thời gian khôi phục",
      en: "Failed deployment recovery time",
      value: recoveries.length ? duration(median(recoveries)) : "—",
      note: `Đã khôi phục: ${recoveries.length}/${incidents.length} sự cố`,
      formula:
        "Trung vị từ sự cố do deployment đến khôi phục. Sự cố chưa khôi phục không có duration và được công bố riêng; không gán bằng 0.",
    },
    {
      name: "Triển khai khắc phục",
      en: "Deployment rework rate",
      value: data.length
        ? `${fmt((data.filter((r) => r.rework).length / data.length) * 100)}%`
        : "—",
      note: `${data.filter((r) => r.rework).length} deployment khắc phục ngoài kế hoạch`,
      formula:
        "Deployment ngoài kế hoạch để khắc phục sự cố production / tổng deployment production. Không phải mọi retry đều là rework.",
    },
  ];
  function csv() {
    const values = [
      ["DỮ LIỆU MẪU — KHÔNG PHẢI BÁO CÁO THỰC TẾ"],
      ["Ứng dụng", app, "Số ngày", String(days)],
      [
        "ID",
        "Ngày",
        "Ứng dụng",
        "Người thực hiện",
        "Phút xử lý yêu cầu",
        "Commit đến production (phút)",
        "Sự cố",
        "Khôi phục (phút)",
        "Khắc phục ngoài kế hoạch",
        "Cần hỗ trợ",
      ],
      ...data.map((r) => [
        r.id,
        date(r.day),
        r.app,
        r.actor,
        String(r.minutes),
        r.lead === null ? "Chưa có dữ liệu" : String(r.lead),
        r.incident ? "Có" : "Không",
        r.recovery === null ? "" : String(r.recovery),
        r.rework ? "Có" : "Không",
        r.support === null ? "Chưa ghi nhận" : r.support ? "Có" : "Không",
      ]),
    ];
    const url = URL.createObjectURL(
      new Blob(
        [
          "\uFEFF" +
            values
              .map((row) =>
                row.map((v) => '"' + v.replaceAll('"', '""') + '"').join(","),
              )
              .join("\r\n"),
        ],
        { type: "text/csv;charset=utf-8" },
      ),
    );
    const a = document.createElement("a");
    a.href = url;
    a.download = `MOCK-hieu-qua-${app}-${days}ngay.csv`;
    a.click();
    URL.revokeObjectURL(url);
    setNotice("Đã xuất CSV.");
  }
  const evidence = (
    <div className="eff-table-wrap">
      <table className="plain-table">
        <thead>
          <tr>
            <th>Deployment / thời điểm</th>
            <th>Người thực hiện</th>
            <th>Commit → production</th>
            <th>Sự cố sau deploy</th>
            <th>Hỗ trợ Ops</th>
          </tr>
        </thead>
        <tbody>
          {data.map((r) => (
            <tr key={r.id}>
              <td>
                <strong>{r.id}</strong>
                <small>{date(r.day)} · production</small>
              </td>
              <td>{r.actor}</td>
              <td>
                {r.lead === null ? (
                  <span className="eff-warning">Thiếu commit</span>
                ) : (
                  duration(r.lead)
                )}
              </td>
              <td>
                {r.incident ? (
                  <span className="eff-warning">
                    {r.recovery === null
                      ? "Chưa khôi phục"
                      : `Khôi phục ${r.recovery} phút`}
                  </span>
                ) : (
                  "Không ghi nhận"
                )}
              </td>
              <td>
                {r.support === null
                  ? "Chưa trả lời"
                  : r.support
                    ? "Có hỗ trợ"
                    : "Tự phục vụ"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
  const selectedMetric = metrics.find((m) => m.name === detail);
  return (
    <div
      className="effectiveness-prototype"
      ref={shell}
      onClickCapture={(e) => {
        const link = (e.target as HTMLElement).closest(".sidebar a");
        if (link && link.getAttribute("href") !== "#effectiveness") {
          e.preventDefault();
          e.stopPropagation();
          setNotice("Mục này chưa có trong bản mẫu.");
        }
      }}
    >
      <AppShell
        username="admin"
        role="ADMIN"
        activeRoute="effectiveness"
        onSignOut={() => setNotice("Bản mẫu không có phiên đăng nhập.")}
      >
        {createPortal(
          <a
            className="nav-item nav-item-active"
            href="#effectiveness"
            aria-current="page"
            onClick={(e) => {
              e.preventDefault();
              setTab("Tổng quan");
            }}
          >
            ▥ <span>Hiệu quả platform</span>
          </a>,
          navSlot,
        )}
        <section className="page eff-page">
          <header className="page-header">
            <div>
              <h1>Hiệu quả platform</h1>
              <span className="eff-demo">Dữ liệu mẫu</span>
            </div>
            <div className="eff-actions">
              <Button onClick={() => setDetail("Mốc so sánh")}>
                Mốc so sánh
              </Button>
              <Button tone="primary" onClick={csv} disabled={!data.length}>
                Xuất CSV
              </Button>
            </div>
          </header>
          <div className="eff-filters">
            <label>
              Ứng dụng
              <select
                aria-label="Ứng dụng"
                value={app}
                onChange={(e) => setApp(e.target.value)}
              >
                <option>checkout-api</option>
                <option>billing-api</option>
              </select>
            </label>
            <label>
              Thời gian
              <select
                aria-label="Thời gian"
                value={days}
                onChange={(e) => setDays(Number(e.target.value))}
              >
                <option value={30}>30 ngày</option>
                <option value={7}>7 ngày</option>
              </select>
            </label>
            <div className="eff-scope">
              <span className="status status-good">
                <span className="status-dot" />
                production
              </span>
              <small>
                {date(24 - days)} — {date(23)}
              </small>
            </div>
            <label className="eff-empty-toggle">
              <input
                type="checkbox"
                checked={empty}
                onChange={(e) => setEmpty(e.target.checked)}
              />
              Chưa có dữ liệu
            </label>
          </div>
          <nav className="tabs" aria-label="Các phần báo cáo">
            {tabs.map((t) => (
              <button
                className={`tab${tab === t ? " tab-active" : ""}`}
                key={t}
                aria-current={tab === t ? "page" : undefined}
                onClick={() => setTab(t)}
              >
                {t}
              </button>
            ))}
          </nav>
          {notice && (
            <div className="form-info eff-notice" role="status">
              {notice}
              <Button
                tone="quiet"
                aria-label="Đóng thông báo"
                onClick={() => setNotice("")}
              >
                ×
              </Button>
            </div>
          )}
          {!data.length ? (
            <div className="empty-state">
              <div className="empty-icon">▥</div>
              <h2>Chưa có dữ liệu</h2>
              <Button onClick={() => setEmpty(false)}>Xem dữ liệu mẫu</Button>
            </div>
          ) : (
            <>
              {tab === "Tổng quan" && (
                <>
                  <div className="section-header">
                    <h2>DORA</h2>
                    <Button
                      tone="quiet"
                      onClick={() => setDetail("Cách đọc chỉ số")}
                    >
                      Cách đọc chỉ số
                    </Button>
                  </div>
                  <div className="eff-metrics">
                    {metrics.map((m, i) => (
                      <button
                        className="eff-metric"
                        key={m.en}
                        onClick={() => setDetail(m.name)}
                        aria-label={`${m.name}: ${m.value}. Xem chi tiết`}
                      >
                        <span className="eff-metric-title">
                          {m.name}
                          <span aria-hidden="true">↗</span>
                        </span>
                        <strong>{m.value}</strong>
                        {i === 1 && leads.length < data.length ? (
                          <small className="eff-warning">
                            Thiếu commit: {data.length - leads.length}/
                            {data.length}
                          </small>
                        ) : i === 3 && recoveries.length < incidents.length ? (
                          <small className="eff-warning">
                            Chưa khôi phục:{" "}
                            {incidents.length - recoveries.length}
                          </small>
                        ) : (
                          <small>
                            {i === 0
                              ? `${data.length} lượt triển khai`
                              : i === 1 || i === 3
                                ? "Trung vị"
                                : m.note.replace(" deployment", " lượt")}
                          </small>
                        )}
                      </button>
                    ))}
                  </div>
                  <div className="eff-two-col">
                    <section className="content-panel">
                      <div className="section-header">
                        <h2>Lượt triển khai</h2>
                        <span className="muted">{data.length} lượt</span>
                      </div>
                      <div
                        className="eff-chart"
                        role="img"
                        aria-label="Số deployment production theo thời gian"
                      >
                        {Array.from({ length: days === 7 ? 7 : 5 }, (_, i) => {
                          const buckets = days === 7 ? 7 : 5;
                          const from =
                              24 - days + Math.floor((i * days) / buckets),
                            to =
                              24 -
                              days +
                              Math.floor(((i + 1) * days) / buckets);
                          const count = data.filter(
                            (r) => r.day >= from && r.day < to,
                          ).length;
                          return (
                            <div className="eff-bar-group" key={i}>
                              <strong>{count}</strong>
                              <div
                                className="eff-bar"
                                style={{
                                  height: `${Math.max(3, count * 13)}px`,
                                }}
                              />
                              <small>{date(from).slice(5)}</small>
                            </div>
                          );
                        })}
                      </div>
                    </section>
                    <section className="content-panel eff-comparison">
                      <div className="section-header">
                        <h2>Thời gian rút ngắn</h2>
                      </div>
                      <strong className="eff-comparison-value">
                        {baseline ? duration(baseline.minutes - current) : "—"}
                      </strong>
                      <p className="muted">
                        {baseline
                          ? `${duration(baseline.minutes)} → ${duration(current)} / yêu cầu`
                          : "Chưa có mốc so sánh"}
                      </p>
                      {baseline && (
                        <small>Thời gian quy trình, không phải giờ công.</small>
                      )}
                      <Button onClick={() => setDetail("Mốc so sánh")}>
                        {baseline ? "Xem mốc so sánh" : "Thêm mốc so sánh"}
                      </Button>
                    </section>
                  </div>
                  <div className="eff-summary">
                    <span>
                      <b>{users}</b> người dùng hoạt động
                    </span>
                    <span>
                      <b>
                        {self}/{answered.length}
                      </b>{" "}
                      lượt tự phục vụ
                    </span>
                    <Button tone="quiet" onClick={() => setTab("Dữ liệu")}>
                      Xem dữ liệu nguồn →
                    </Button>
                  </div>
                </>
              )}
              {tab === "Tự phục vụ" && (
                <>
                  <div className="eff-usage-grid">
                    {[
                      {
                        name: "Người dùng hoạt động",
                        value: String(users),
                        note: "Có triển khai trong kỳ",
                      },
                      {
                        name: "Tự phục vụ",
                        value: answered.length
                          ? `${fmt((self / answered.length) * 100)}%`
                          : "—",
                        note: `${self}/${answered.length} lượt đã phản hồi`,
                      },
                      {
                        name: "Chưa phản hồi hỗ trợ",
                        value: String(data.length - answered.length),
                        note: `Trên ${data.length} lượt triển khai`,
                      },
                    ].map((m) => (
                      <button
                        className="eff-metric"
                        key={m.name}
                        onClick={() => setDetail("Tự phục vụ")}
                      >
                        <span className="eff-metric-title">
                          {m.name}
                          <span aria-hidden="true">↗</span>
                        </span>
                        <strong>{m.value}</strong>
                        <small>{m.note}</small>
                      </button>
                    ))}
                  </div>
                  <section className="content-panel eff-pending">
                    <h2>Cần bổ sung dữ liệu</h2>
                    <div>
                      <span>Công sức Ops</span>
                      <span className="muted">Chưa thu thập</span>
                    </div>
                    <div>
                      <span>Thời gian onboarding</span>
                      <span className="muted">Chưa thu thập</span>
                    </div>
                    <Button
                      tone="quiet"
                      onClick={() => setDetail("Tự phục vụ")}
                    >
                      Cách ghi nhận
                    </Button>
                  </section>
                </>
              )}
              {tab === "Dữ liệu" && (
                <section className="content-panel">
                  <div className="section-header">
                    <h2>Lịch sử triển khai</h2>
                    <span className="muted">{data.length} lượt</span>
                  </div>
                  {evidence}
                </section>
              )}
            </>
          )}
        </section>
      </AppShell>
      <dialog
        className="eff-dialog"
        ref={dialog}
        aria-labelledby="eff-dialog-title"
        onCancel={() => setDetail(null)}
        onClick={(e) => {
          if (e.target === e.currentTarget) setDetail(null);
        }}
      >
        <div className="section-header">
          <div>
            <h2 id="eff-dialog-title">{detail}</h2>
            {selectedMetric && <p>{selectedMetric.en}</p>}
          </div>
          <Button
            tone="quiet"
            aria-label="Đóng chi tiết"
            onClick={() => setDetail(null)}
          >
            ×
          </Button>
        </div>
        {detail === "Mốc so sánh" ? (
          <form
            className="editor-grid"
            onSubmit={(e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              setBaseline({
                minutes: Number(f.get("minutes")),
                source: String(f.get("source")).trim(),
              });
              setNotice("Đã lưu mốc so sánh mẫu.");
              setDetail(null);
            }}
          >
            <label>
              Thời gian hoàn thành yêu cầu (phút)
              <input
                name="minutes"
                type="number"
                min="1"
                max="100000"
                required
                defaultValue={baseline?.minutes}
              />
            </label>
            <label>
              Nguồn đo
              <textarea
                name="source"
                required
                minLength={5}
                defaultValue={baseline?.source}
                placeholder="Phương pháp, số mẫu và phạm vi áp dụng"
              />
            </label>
            <small className="muted">
              Mẫu dùng chung cho hai ứng dụng; chỉ lưu đến khi tải lại trang.
            </small>
            <details>
              <summary>Cách so sánh</summary>
              <p>
                So cùng loại yêu cầu và tiêu chí hoàn thành. Lấy thời gian thủ
                công trừ trung vị thời gian yêu cầu qua platform. Đây là thời
                gian quy trình rút ngắn, không phải giờ công tiết kiệm. Bản thật
                cần mốc đo theo ứng dụng và loại công việc.
              </p>
              <p>
                <a
                  href="https://docs.port.io/guides/all/create-roi-dashboard/"
                  target="_blank"
                  rel="noreferrer"
                >
                  Port · Phương pháp đo ↗
                </a>
              </p>
            </details>
            <div className="eff-actions">
              <Button tone="primary" type="submit">
                Lưu mốc so sánh
              </Button>
              {baseline && (
                <Button
                  type="button"
                  onClick={() => {
                    setBaseline(null);
                    setDetail(null);
                    setNotice("Đã xóa mốc so sánh.");
                  }}
                >
                  Xóa mốc so sánh
                </Button>
              )}
            </div>
          </form>
        ) : detail === "Cách đọc chỉ số" ? (
          <div className="eff-help">
            <p>
              DORA đo tốc độ và độ ổn định khi giao phần mềm. Các chỉ số ở đây
              áp dụng cho <b>{app}</b> trên <b>production</b>.
            </p>
            <ul>
              {metrics.map((m) => (
                <li key={m.en}>
                  <b>{m.name}</b>
                  <p>{m.formula}</p>
                </li>
              ))}
            </ul>
            <a
              href="https://dora.dev/guides/dora-metrics/"
              target="_blank"
              rel="noreferrer"
            >
              Định nghĩa DORA ↗
            </a>
          </div>
        ) : detail === "Tự phục vụ" ? (
          <div className="eff-help">
            <p>
              Tỷ lệ tự phục vụ = số lượt xác nhận không cần hỗ trợ / số lượt đã
              trả lời. Chưa trả lời không được tính là tự phục vụ.
            </p>
            <p>Người dùng hoạt động: có ít nhất một deployment trong kỳ.</p>
            <p>
              Công sức Ops cần số phút hỗ trợ và bảo trì được ghi nhận riêng.
              Onboarding cần mốc bắt đầu và lần triển khai thành công đầu tiên.
              Chưa có dữ liệu thì không tính thành 0.
            </p>
            <a
              href="https://humanitec.com/blog/impact-of-internal-developer-platforms"
              target="_blank"
              rel="noreferrer"
            >
              Humanitec · Chỉ số tham khảo ↗
            </a>
          </div>
        ) : (
          <>
            <p>{selectedMetric?.formula}</p>
            <p className="muted">
              {selectedMetric?.note} · {app} · production
            </p>
            {evidence}
          </>
        )}
      </dialog>
    </div>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
