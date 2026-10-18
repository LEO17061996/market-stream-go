import Link from "next/link";

export default function NotFound() {
  return (
    <main className="mx-auto max-w-md p-8 text-center">
      <h1 className="mb-2 text-lg font-semibold">Không tìm thấy trang</h1>
      <Link href="/" className="text-[var(--accent)]">
        Về bảng giá
      </Link>
    </main>
  );
}
