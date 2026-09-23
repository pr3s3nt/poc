import type { ButtonHTMLAttributes, PropsWithChildren } from 'react';

type ButtonProps = PropsWithChildren<ButtonHTMLAttributes<HTMLButtonElement>> & {
  tone?: 'primary' | 'secondary' | 'danger' | 'quiet';
};

export function Button({ tone = 'secondary', className = '', children, ...props }: ButtonProps) {
  return <button className={`button button-${tone} ${className}`.trim()} {...props}>{children}</button>;
}
