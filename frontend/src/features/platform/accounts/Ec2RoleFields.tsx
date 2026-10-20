import { useId } from 'react';

export function Ec2RoleFields({ roleArn, regions, disabled, onRoleArn, onRegions }: {
  roleArn: string; regions: string; disabled: boolean; onRoleArn(value: string): void; onRegions(value: string): void;
}) {
  const id = useId();
  return <>
    <p className="muted">Backend và executor phải chạy trên EC2 được gắn role này. Hệ thống dùng credential tạm thời từ EC2 và tự làm mới khi chạy Terraform; bạn không cần nhập access key.</p>
    <label className="connection-field">IAM Role ARN<input value={roleArn} disabled={disabled} required autoComplete="off" spellCheck={false} maxLength={2048} placeholder="arn:aws:iam::123456789012:role/OrchestratorRole" aria-describedby={`${id}-role`} onChange={(event) => onRoleArn(event.target.value)} /></label>
    <small id={`${id}-role`} className="muted">Nhập ARN của IAM Role dự kiến gắn với EC2, không dùng ARN phiên assumed-role. Hệ thống từ chối nếu role thực tế không khớp.</small>
    <label className="connection-field">Vùng kiểm tra<input value={regions} disabled={disabled} required autoComplete="off" spellCheck={false} placeholder="us-east-1, eu-west-1" aria-describedby={`${id}-regions`} onChange={(event) => onRegions(event.target.value)} /></label>
    <small id={`${id}-regions`} className="muted">Tối đa 10 vùng để kiểm tra kết nối và quyền. Vùng triển khai được chọn ở Driver Inputs.</small>
  </>;
}
