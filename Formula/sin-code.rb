class SinCode < Formula
  desc "SIN-Code unified toolchain — 13 analysis and manipulation tools in one binary"
  homepage "https://github.com/OpenSIN-Code/SIN-Code-Bundle"
  version "1.0.2"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.2-sin-code/sin-code-darwin-arm64.tar.gz"
      sha256 "a562dace23f609f3a7e615c2dacf84ca62eb7538f5e976131b5a4977b167c44d"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.2-sin-code/sin-code-darwin-amd64.tar.gz"
      sha256 "4709870c44604bf0cdb42793395b653d08a0335334665dc19ef8f3e7b6b1537f"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.2-sin-code/sin-code-linux-arm64.tar.gz"
      sha256 "708519f3c7d8e47468d783205950ac19c7f8b118a974e02cfafcdf938244f4d1"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.2-sin-code/sin-code-linux-amd64.tar.gz"
      sha256 "d3df6b46b4436dab6b7481ee1942fc4fee147b608d27c01367fbfcb254d11f05"
    end
  end

  def install
    bin.install "sin-code"
  end

  test do
    system "#{bin}/sin-code", "--version"
  end
end
