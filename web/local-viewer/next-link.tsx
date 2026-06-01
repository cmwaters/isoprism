import type { AnchorHTMLAttributes, ReactNode } from "react";

// LinkProps describes the props consumed by this component.
type LinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
  href: string;
  prefetch?: boolean;
  children?: ReactNode;
};

// Link adapts anchor tags to the Next Link interface expected by shared components.
export default function Link({ href, prefetch: _prefetch, children, ...props }: LinkProps) {
  return (
    <a href={href} {...props}>
      {children}
    </a>
  );
}
