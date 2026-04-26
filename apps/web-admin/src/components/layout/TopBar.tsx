export function TopBar() {
  return (
    <header className="bg-white border-b border-gray-200 px-6 py-3 flex items-center justify-between shrink-0">
      <div className="text-sm text-gray-500">Панель сотрудника</div>
      <div className="flex items-center gap-3">
        <div className="w-8 h-8 bg-primary/10 rounded-full flex items-center justify-center text-primary text-sm font-semibold">А</div>
        <span className="text-sm font-medium text-gray-700">Администратор</span>
      </div>
    </header>
  );
}
