/** The Vanish mark: a V whose second stroke breaks up and fades away. */
export const Logo = ({ className }: { className?: string }) => (
  <svg
    viewBox="0 0 24 24"
    className={className}
    aria-hidden="true"
    fill="currentColor"
  >
    <path
      d="M3.6 4.6 11 18.4"
      stroke="currentColor"
      strokeWidth="3.2"
      strokeLinecap="round"
      fill="none"
    />
    <circle cx="13.9" cy="14.1" r="1.75" />
    <circle cx="16.4" cy="9.9" r="1.35" />
    <circle cx="18.5" cy="6.4" r="1" />
    <circle cx="20.2" cy="3.6" r="0.65" />
  </svg>
);
