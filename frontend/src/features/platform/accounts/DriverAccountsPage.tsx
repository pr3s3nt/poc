import { useEffect, useRef, useState } from 'react';
import { replaceRoute } from '../../../app/routes';
import { Button } from '../../../shared/ui/Button';
import { useCatalogList } from '../useCatalogList';
import { AccountDetail } from './AccountDetail';
import { CreateAccountForm } from './CreateAccountForm';
import { accountGuidance, authLabel, getAccount, listAccounts, statusLabel, typeLabel, type Account } from './accountsApi';

const listFailure = () => 'Không tải được danh sách tài khoản hạ tầng. Kiểm tra kết nối mạng rồi thử lại; dữ liệu trong form được giữ nguyên.';

function destination(account: Account): string {
  const config = account.config ?? {};
  return account.type === 'AWS'
    ? [config.accountId, (config.verificationRegions ?? []).length ? `vùng kiểm tra: ${config.verificationRegions?.join(', ')}` : ''].filter(Boolean).join(' · ')
    : [config.cluster, config.endpoint].filter(Boolean).join(' · ');
}

/**
 * V01 infrastructure accounts: AWS and Internal, version-guarded management
 * over the same backing records as the legacy Connections page. No Environment
 * assignment and no applicability controls exist here.
 */
export function DriverAccountsPage({ initialAccount }: { initialAccount?: string }) {
  const { items, loading, loadError, reload } = useCatalogList(listAccounts, listFailure);
  const [selectedKey, setSelectedKey] = useState(initialAccount ?? '');
  const [detail, setDetail] = useState<Account | null>(null);
  const [detailState, setDetailState] = useState<'idle' | 'loading' | 'error'>('idle');
  const [detailError, setDetailError] = useState('');
  const [listNotice, setListNotice] = useState('');
  const request = useRef(0);

  async function loadDetail(key: string) {
    const id = ++request.current;
    setDetail(null); setDetailState('loading'); setDetailError('');
    try {
      const account = await getAccount(key);
      if (id !== request.current) return;
      setDetail(account); setDetailState('idle');
    } catch (reason) {
      if (id !== request.current) return;
      setDetailState('error'); setDetailError(accountGuidance('rename', reason));
    }
  }

  useEffect(() => {
    if (!selectedKey) { request.current += 1; setDetail(null); setDetailState('idle'); return; }
    void loadDetail(selectedKey);
  }, [selectedKey]);
  useEffect(() => () => { request.current += 1; }, []);

  function select(key: string) {
    setSelectedKey(key);
    replaceRoute({ name: 'driver-accounts', ...(key ? { account: key } : {}) });
  }

  // A committed change updates the detail at once; the list refresh is a
  // separate request whose failure never contradicts the committed result.
  function changed(account: Account) {
    setDetail(account);
    setListNotice('');
    void reload();
  }

  async function created(account: Account) {
    setListNotice('');
    await reload();
    select(account.key);
  }

  const visible = items;
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Tài khoản hạ tầng</h1><p>Danh mục tài khoản AWS và cluster Internal của tổ chức. Tài khoản chỉ lưu danh tính và đích kết nối; chúng không gán cho môi trường nào và không tạo tài nguyên. Thông tin xác thực nằm trong kho credential của nền tảng và không bao giờ được đọc lại.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Tài khoản đã đăng ký</h2><Button type="button" onClick={() => { setListNotice(''); void reload(); }} disabled={loading}>Tải lại danh sách</Button></div>
      {loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải danh sách">{loadError} <Button type="button" onClick={() => void reload()}>Thử lại</Button></div> : null}
      {listNotice ? <div className="form-success" role="status">{listNotice}</div> : null}
      {loading && visible.length === 0 ? <p>Đang tải danh sách tài khoản…</p> : !loadError && visible.length === 0 ? <p>Chưa có tài khoản hạ tầng nào được đăng ký.</p> : null}
      {visible.length > 0 ? <table className="connection-table" aria-label="Tài khoản hạ tầng">
        <thead><tr><th scope="col">Tên</th><th scope="col">Loại hạ tầng</th><th scope="col">Xác thực</th><th scope="col">Đích</th><th scope="col">Trạng thái</th><th scope="col">Phiên bản</th></tr></thead>
        <tbody>{visible.map((account) => <tr key={account.key} className={account.key === selectedKey ? 'row-selected' : undefined}>
          <td data-label="Tên"><button type="button" className="link-button" aria-label={`Xem ${account.name}`} aria-pressed={account.key === selectedKey} onClick={() => select(account.key)}><strong>{account.name}</strong></button>{account.name !== account.key ? <small className="connection-key">{account.key}</small> : null}</td>
          <td data-label="Loại hạ tầng">{typeLabel(account.type)}</td>
          <td data-label="Xác thực">{authLabel(account.authenticationType)}</td>
          <td data-label="Đích">{destination(account) || <span className="muted">—</span>}</td>
          <td data-label="Trạng thái"><span className={`status-badge${account.status === 'READY' ? ' status-badge-ready' : ''}`}>{statusLabel(account.status)}</span></td>
          <td data-label="Phiên bản">{account.version}</td>
        </tr>)}</tbody>
      </table> : null}
    </section>
    {selectedKey ? detailState === 'loading' ? <section className="content-panel" role="status" aria-busy="true">Đang tải chi tiết tài khoản…</section>
      : detailState === 'error' ? <section className="content-panel"><div className="form-error" role="alert">{detailError} <Button type="button" onClick={() => void loadDetail(selectedKey)}>Thử lại</Button> <Button type="button" onClick={() => select('')}>Đóng</Button></div></section>
      : detail ? <AccountDetail key={detail.key} account={detail} onChanged={changed} onRefreshed={(account) => { setDetail(account); void reload(); }} /> : null : null}
    <section className="content-panel"><div className="connection-register">
      <div className="section-header"><div><h2>Đăng ký tài khoản mới</h2><p>Chọn loại hạ tầng và cách xác thực. Mỗi lần lưu chỉ tạo một tài khoản mới; không ghi đè tài khoản có sẵn.</p></div></div>
      <CreateAccountForm onCreated={created} onUnknownOutcome={() => { setListNotice('Chưa biết lần đăng ký trước đã được lưu hay chưa; đang tải lại danh sách để kiểm tra. Console không tự gửi lại.'); return reload(); }} />
    </div></section>
  </section>;
}
