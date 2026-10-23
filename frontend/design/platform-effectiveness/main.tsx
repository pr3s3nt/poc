import { useRef, useState, useEffect } from "react";
import { createRoot } from "react-dom/client";
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
const tabs = ["Tổng quan", "DORA", "Tự phục vụ & sử dụng", "Baseline"];
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
      value: data.length ? `${fmt(data.length / days)} / ngày` : "—",
      note: `${data.length} deployment production / ${days} ngày`,
      formula:
        "Số deployment đã lên production / số ngày trong kỳ. Các bản ghi mẫu đều đã lên production; sự cố sau deploy được ghi riêng.",
    },
    {
      name: "Thời gian đưa thay đổi lên production",
      en: "Change lead time",
      value: leads.length ? duration(median(leads)) : "—",
      note: `Có liên kết commit: ${leads.length}/${data.length}`,
      formula:
        "Trung vị thời gian từ commit đến deployment production thành công. Không dùng thời gian chạy pipeline thay thế. Chỉ tính bản ghi có commit.",
    },
    {
      name: "Tỷ lệ thay đổi gây lỗi",
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
      name: "Tỷ lệ triển khai khắc phục",
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
    setNotice("Đã xuất CSV dữ liệu mẫu theo bộ lọc hiện tại.");
  }
  const evidence = (
    <div className="table-wrap">
      <table>
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
                  <span className="amber">Thiếu commit</span>
                ) : (
                  duration(r.lead)
                )}
              </td>
              <td>
                {r.incident ? (
                  <span className="amber">
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
  return (
    <>
      <aside>
        <div className="brand">
          <span className="brand-icon">O</span> orchestrator
          <span className="brand-dot">.</span>
        </div>
        <div className="workspace">
          AC{" "}
          <span>
            Acme Engineering<small>Tổ chức minh họa</small>
          </span>
        </div>
        <div className="nav-label">WORKSPACE</div>
        <div className="nav-muted">▧ Ứng dụng</div>
        <div className="nav-muted">↻ Lịch sử triển khai</div>
        <div className="nav-active">
          ▥ Hiệu quả platform <span>Mới</span>
        </div>
        <div className="nav-label">NỀN TẢNG</div>
        <div className="nav-muted">◇ Loại tài nguyên</div>
        <div className="nav-muted">▦ Cấu hình tài nguyên</div>
        <div className="nav-muted">⊞ Mẫu dựng ứng dụng</div>
        <div className="nav-muted">⇄ Kết nối hạ tầng</div>
        <div className="nav-muted">♧ Kho bí mật</div>
        <div className="sidebar-bottom">
          <span className="avatar">AD</span>
          <div>
            Quản trị viên<small>Phiên duyệt giao diện</small>
          </div>
        </div>
      </aside>
      <div className="shell">
        <header>
          <span>
            Workspace <span className="slash">/</span> <b>Hiệu quả platform</b>
          </span>
          <span className="prototype">BẢN THIẾT KẾ · 01</span>
        </header>
        <main>
          <div className="heading">
            <div>
              <div className="eyebrow">ĐO LƯỜNG GIÁ TRỊ NỀN TẢNG</div>
              <h1>Hiệu quả platform</h1>
              <p>Từ hoạt động thực tế đến những cải thiện có thể đo lường.</p>
            </div>
            <button className="primary" onClick={csv} disabled={!data.length}>
              ↓ Xuất báo cáo CSV
            </button>
          </div>
          <div className="demo">
            <span>
              <b>Dữ liệu minh họa</b> · Dùng để duyệt giao diện, không phản ánh
              hiệu quả dự án.
            </span>
            <label>
              <input
                type="checkbox"
                checked={empty}
                onChange={(e) => setEmpty(e.target.checked)}
              />{" "}
              Xem khi chưa có dữ liệu
            </label>
          </div>
          <div className="filters">
            <label>
              Ứng dụng
              <select aria-label="Ứng dụng" value={app} onChange={(e) => setApp(e.target.value)}>
                <option>checkout-api</option>
                <option>billing-api</option>
              </select>
            </label>
            <label>
              Khoảng thời gian
              <select
                aria-label="Khoảng thời gian"
                value={days}
                onChange={(e) => setDays(Number(e.target.value))}
              >
                <option value={30}>30 ngày gần nhất</option>
                <option value={7}>7 ngày gần nhất</option>
              </select>
            </label>
            <div>
              <small>Môi trường đo lường</small>
              <b>
                <span className="dot" /> production
              </b>
            </div>
            <span className="filter-date">
              {date(24 - days)} — 23/10/2026
              <small>Mốc thời gian cố định của bản mẫu · UTC</small>
            </span>
          </div>
          <nav className="tabs" aria-label="Các phần báo cáo">
            {tabs.map((t) => (
              <button
                key={t}
                aria-current={tab === t ? "page" : undefined}
                onClick={() => setTab(t)}
              >
                {t}
              </button>
            ))}
          </nav>
          {notice && (
            <div role="status" className="notice">
              {notice}
              <button aria-label="Đóng thông báo" onClick={() => setNotice("")}>
                ×
              </button>
            </div>
          )}
          {empty && tab !== "Baseline" ? (
            <section className="empty">
              <div>▥</div>
              <h2>Chưa có dữ liệu trong kỳ này</h2>
              <p>
                Khi có hoạt động thực tế, báo cáo sẽ xuất hiện tại đây.
                <br />
                Chỉ số chưa có dữ liệu không được tính là 0.
              </p>
              <button onClick={() => setEmpty(false)}>
                Xem dữ liệu minh họa
              </button>
            </section>
          ) : (
            <>
              {(tab === "Tổng quan" || tab === "DORA") && (
                <>
                  <div className="section-title">
                    <div>
                      <h2>
                        {tab === "DORA"
                          ? "Hiệu quả giao phần mềm"
                          : "Giao phần mềm nhanh và ổn định hơn"}
                      </h2>
                      <p>
                        5 chỉ số DORA · Theo từng ứng dụng, không xếp hạng đội
                        ngũ.
                      </p>
                    </div>
                    <a
                      href="https://dora.dev/guides/dora-metrics/"
                      target="_blank"
                      rel="noreferrer"
                    >
                      Định nghĩa DORA ↗
                    </a>
                  </div>
                  <div className="metrics">
                    {metrics.map((m, i) => (
                      <button
                        className="metric"
                        key={m.en}
                        onClick={() => setDetail(m.name)}
                      >
                        <span className="metric-index">
                          0{i + 1}
                          <span>↗</span>
                        </span>
                        <h3>{m.name}</h3>
                        <small>{m.en}</small>
                        <div className="metric-value">{m.value}</div>
                        <div className="metric-note">{m.note}</div>
                      </button>
                    ))}
                  </div>
                  {tab === "Tổng quan" && (
                    <div className="two-col">
                      <section>
                        <div className="section-title">
                          <div>
                            <h2>Nhịp triển khai</h2>
                            <p>
                              Số deployment production theo khoảng thời gian
                            </p>
                          </div>
                          <span className="pill">{data.length} deployment</span>
                        </div>
                        <div
                          className="chart"
                          role="img"
                          aria-label="Biểu đồ số deployment theo khoảng thời gian"
                        >
                          {Array.from(
                            { length: days === 7 ? 7 : 5 },
                            (_, i) => {
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
                                <div className="bar-group" key={i}>
                                  <strong>{count}</strong>
                                  <div
                                    className="bar"
                                    style={{
                                      height: `${Math.max(3, count * 12)}px`,
                                    }}
                                  />
                                  <small>
                                    {date(from).slice(5)}
                                    {days === 30
                                      ? ` – ${date(to - 1).slice(5)}`
                                      : ""}
                                  </small>
                                </div>
                              );
                            },
                          )}
                        </div>
                        <div className="chart-footer">
                          <span className="dot" /> Deployment mẫu · cùng ứng
                          dụng và môi trường
                        </div>
                      </section>
                      <section className="value-panel">
                        <div className="eyebrow">SO SÁNH TRƯỚC & SAU</div>
                        <h2>
                          Platform đã rút ngắn
                          <br />
                          bao nhiêu thời gian?
                        </h2>
                        {baseline ? (
                          <>
                            <div className="hero-number">
                              {duration(baseline.minutes - current)}
                            </div>
                            <p>
                              Chênh lệch thời gian hoàn thành yêu cầu điển hình.
                              <br />
                              Baseline {duration(baseline.minutes)} → hiện tại{" "}
                              {duration(current)}.
                            </p>
                          </>
                        ) : (
                          <>
                            <div className="hero-number">Chưa có baseline</div>
                            <p>
                              Bổ sung mốc đo trước khi sử dụng platform để có
                              phép so sánh có căn cứ.
                            </p>
                          </>
                        )}
                        <div className="callout">
                          Thời gian quy trình rút ngắn không đồng nghĩa với giờ
                          công tiết kiệm.
                        </div>
                        <button onClick={() => setTab("Baseline")}>
                          {baseline ? "Xem baseline" : "Thiết lập baseline"} →
                        </button>
                      </section>
                    </div>
                  )}
                  <section className="quality">
                    <div>
                      <h3>Độ đầy đủ của dữ liệu</h3>
                      <p>
                        {leads.length}/{data.length} deployment có commit ·{" "}
                        {recoveries.length}/{incidents.length} sự cố có mốc khôi
                        phục. Các chỉ số chỉ tính trên bản ghi đủ dữ liệu.
                      </p>
                    </div>
                    <button onClick={() => setDetail("Nguồn dữ liệu")}>
                      Kiểm tra nguồn →
                    </button>
                  </section>
                  {tab === "DORA" && (
                    <section>
                      <h2>Lịch sử làm căn cứ</h2>
                      <p>Các bản ghi giả lập đứng sau chỉ số ở trên.</p>
                      {evidence}
                    </section>
                  )}
                </>
              )}
              {tab === "Tự phục vụ & sử dụng" && (
                <>
                  <div className="section-title">
                    <div>
                      <h2>Giảm phụ thuộc, mở rộng khả năng phục vụ</h2>
                      <p>
                        Đo mức sử dụng và nhu cầu hỗ trợ trong phạm vi đã chọn.
                      </p>
                    </div>
                  </div>
                  <div className="usage-grid">
                    {[
                      {
                        name: "Người dùng hoạt động",
                        value: String(users),
                        note: "Có ít nhất một deployment trong kỳ",
                      },
                      {
                        name: "Deployment production",
                        value: String(data.length),
                        note: "Số lượt thực hiện, không phải giờ công",
                      },
                      {
                        name: "Tỷ lệ tự phục vụ",
                        value: answered.length
                          ? `${fmt((self / answered.length) * 100)}%`
                          : "—",
                        note: `${self}/${answered.length} lượt đã trả lời không cần hỗ trợ`,
                      },
                      {
                        name: "Độ phủ phản hồi hỗ trợ",
                        value: data.length
                          ? `${fmt((answered.length / data.length) * 100)}%`
                          : "—",
                        note: `${data.length - answered.length} lượt chưa trả lời`,
                      },
                    ].map((m) => (
                      <button
                        className="metric"
                        key={m.name}
                        onClick={() => setDetail("Nguồn dữ liệu")}
                      >
                        <h3>{m.name}</h3>
                        <div className="metric-value">{m.value}</div>
                        <p>{m.note}</p>
                      </button>
                    ))}
                  </div>
                  <div className="two-col">
                    <section>
                      <h2>Công sức Ops hỗ trợ & bảo trì</h2>
                      <div className="missing">Chưa thu thập</div>
                      <p>
                        Cần dữ liệu giờ công thực tế. Không suy ra từ thời gian
                        chạy tự động hay vai trò người bấm deploy.
                      </p>
                    </section>
                    <section>
                      <h2>Thời gian onboarding</h2>
                      <div className="missing">Chưa thu thập</div>
                      <p>
                        Dự kiến đo từ lúc bắt đầu onboarding đến lần triển khai
                        thành công đầu tiên. Cần ghi nhận mốc bắt đầu.
                      </p>
                    </section>
                  </div>
                  <section>
                    <h2>Ghi nhận tự phục vụ</h2>
                    <p>
                      Phản hồi minh họa: có / không cần hỗ trợ / chưa trả lời.
                    </p>
                    {evidence}
                  </section>
                </>
              )}
              {tab === "Baseline" && (
                <div className="baseline-layout">
                  <section>
                    <div className="eyebrow">MỐC ĐỐI CHIẾU</div>
                    <h2>Thời gian xử lý yêu cầu triển khai</h2>
                    <p>
                      Nhập sau khi bạn đo quy trình thủ công. Bản mẫu dùng cùng
                      baseline cho hai ứng dụng để duyệt tương tác; bản thật cần
                      baseline đúng phạm vi.
                    </p>
                    <form
                      onSubmit={(e) => {
                        e.preventDefault();
                        const f = new FormData(e.currentTarget);
                        setBaseline({
                          minutes: Number(f.get("minutes")),
                          source: String(f.get("source")).trim(),
                        });
                        setNotice(
                          "Đã lưu baseline mẫu trong phiên duyệt. Tải lại trang sẽ đặt lại.",
                        );
                      }}
                    >
                      <label>
                        Thời gian từ gửi yêu cầu đến hoàn thành (phút)
                        <input
                          name="minutes"
                          type="number"
                          min="1"
                          max="100000"
                          required
                          defaultValue={baseline?.minutes}
                          placeholder="Ví dụ: nhập kết quả bạn đã đo"
                        />
                      </label>
                      <label>
                        Nguồn / phương pháp đo
                        <textarea
                          name="source"
                          required
                          minLength={5}
                          defaultValue={baseline?.source}
                          placeholder="Đợt khảo sát, số mẫu, quy trình và phạm vi áp dụng…"
                        />
                      </label>
                      <div className="callout">
                        Chỉ lưu trong phiên mock UI. Không ghi database và không
                        thay đổi dữ liệu thật.
                      </div>
                      <button className="primary" type="submit">
                        Lưu baseline mẫu
                      </button>
                      {baseline && (
                        <button
                          type="button"
                          onClick={() => {
                            setBaseline(null);
                            setNotice("Đã xóa baseline mẫu.");
                          }}
                        >
                          Xóa baseline mẫu
                        </button>
                      )}
                    </form>
                  </section>
                  <section className="baseline-guide">
                    <h2>Đọc kết quả đúng cách</h2>
                    <ol>
                      <li>
                        <b>Cùng công việc, cùng phạm vi</b>
                        <p>So sánh cùng loại yêu cầu và tiêu chí hoàn thành.</p>
                      </li>
                      <li>
                        <b>Giữ nguồn đo</b>
                        <p>
                          Baseline phải có căn cứ và lịch sử phiên bản trong bản
                          thật.
                        </p>
                      </li>
                      <li>
                        <b>Phân biệt thời gian và công sức</b>
                        <p>
                          Chưa có dữ liệu giờ công thì chưa báo cáo tiền hoặc
                          ROI.
                        </p>
                      </li>
                    </ol>
                    <hr />
                    <p>Tham khảo phương pháp</p>
                    <a
                      href="https://docs.port.io/guides/all/create-roi-dashboard/"
                      target="_blank"
                      rel="noreferrer"
                    >
                      Port · Action ROI ↗
                    </a>
                    <br />
                    <a
                      href="https://humanitec.com/blog/impact-of-internal-developer-platforms"
                      target="_blank"
                      rel="noreferrer"
                    >
                      Humanitec · Platform impact ↗
                    </a>
                  </section>
                </div>
              )}
            </>
          )}
          <footer>
            <span>Orchestrator / Hiệu quả platform</span>
            <span>Dữ liệu mẫu · Chưa kết nối nguồn thực tế</span>
          </footer>
        </main>
      </div>
      <dialog
        ref={dialog}
        onCancel={() => setDetail(null)}
        onClick={(e) => {
          if (e.target === e.currentTarget) setDetail(null);
        }}
      >
        <div className="dialog-heading">
          <div>
            <div className="eyebrow">MINH BẠCH SỐ LIỆU</div>
            <h2>{detail}</h2>
          </div>
          <button aria-label="Đóng chi tiết" onClick={() => setDetail(null)}>
            ×
          </button>
        </div>
        <div className="callout">
          {metrics.find((m) => m.name === detail)?.formula ??
            "Nguồn: tập dữ liệu giả lập trong mock UI. Bộ lọc ứng dụng và thời gian được áp dụng cho cả chỉ số và danh sách này."}
        </div>
        <p>
          {app} · production · {data.length} deployment mẫu · {date(24 - days)}{" "}
          đến 23/10/2026
        </p>
        {evidence}
        <p className="muted">
          Không có sự kiện thật hoặc bằng chứng hiệu quả kinh doanh trong bản
          mẫu này.
        </p>
      </dialog>
    </>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
