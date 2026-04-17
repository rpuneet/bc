import { Nav } from "../_components/Nav";
import { Footer } from "../_components/Footer";

export default function PrivacyPolicy() {
  return (
    <main className="min-h-screen bg-background">
      <Nav />

      <div className="mx-auto max-w-3xl px-6 pt-24 pb-16 lg:pt-32">
        <article>
          <header className="mb-12">
            <span className="inline-block rounded-full border border-primary/20 bg-primary/5 px-3 py-1 text-xs font-mono font-bold text-primary mb-4">
              LEGAL
            </span>
            <h1 className="text-4xl font-bold tracking-tight mb-2">
              Privacy Policy
            </h1>
            <p className="text-sm text-muted-foreground font-mono">
              Last updated: February 2026
            </p>
          </header>

          <section className="space-y-8 text-[15px] leading-relaxed">
            <div>
              <h2 className="text-xl font-bold mb-3">Introduction</h2>
              <p className="text-muted-foreground">
                mycel (&ldquo;we,&rdquo; &ldquo;us,&rdquo; &ldquo;our,&rdquo; or
                &ldquo;Company&rdquo;) is committed to protecting your privacy.
                This Privacy Policy explains how we collect, use, disclose, and
                safeguard your information when you visit our website and use
                our services.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">
                1. Information We Collect
              </h2>
              <p className="text-muted-foreground mb-3">
                We may collect information about you in a variety of ways. The
                information we may collect on the Site includes:
              </p>
              <ul className="list-disc list-inside space-y-2 text-muted-foreground">
                <li>
                  <strong className="text-foreground">Personal Data:</strong>{" "}
                  Name, email address, phone number, and other information you
                  voluntarily provide
                </li>
                <li>
                  <strong className="text-foreground">Usage Data:</strong>{" "}
                  Information about how you interact with our website, including
                  pages visited, time spent, and referral sources
                </li>
                <li>
                  <strong className="text-foreground">
                    Device Information:
                  </strong>{" "}
                  Browser type, IP address, operating system, and device
                  identifiers
                </li>
                <li>
                  <strong className="text-foreground">Cookies:</strong> We use
                  cookies to enhance your experience and collect analytics data
                </li>
              </ul>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">
                2. How We Use Your Information
              </h2>
              <p className="text-muted-foreground mb-3">
                We use the information we collect in the following ways:
              </p>
              <ul className="list-disc list-inside space-y-2 text-muted-foreground">
                <li>To provide and maintain our services</li>
                <li>To notify you about changes to our services</li>
                <li>
                  To allow you to participate in interactive features of our
                  site
                </li>
                <li>To provide customer support and respond to inquiries</li>
                <li>To gather analysis and feedback to improve our services</li>
                <li>To send promotional communications (with your consent)</li>
              </ul>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">3. Data Security</h2>
              <p className="text-muted-foreground">
                We implement appropriate technical and organizational measures
                to protect your personal information against unauthorized
                access, alteration, disclosure, or destruction. However, no
                method of transmission over the Internet is 100% secure.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">
                4. Third-Party Services
              </h2>
              <p className="text-muted-foreground">
                Our website may contain links to third-party websites and
                services that are not operated by us. This Privacy Policy does
                not apply to third-party websites, and we are not responsible
                for their privacy practices.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">
                5. Children&apos;s Privacy
              </h2>
              <p className="text-muted-foreground">
                Our services are not directed to children under 13 years of age,
                and we do not knowingly collect personal information from
                children under 13. If we become aware that we have collected
                personal information from a child under 13, we will take steps
                to delete such information.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">6. Your Rights</h2>
              <p className="text-muted-foreground mb-3">
                Depending on your location, you may have certain rights
                regarding your personal information, including:
              </p>
              <ul className="list-disc list-inside space-y-2 text-muted-foreground">
                <li>The right to access your personal information</li>
                <li>
                  The right to correct or update your personal information
                </li>
                <li>
                  The right to request deletion of your personal information
                </li>
                <li>
                  The right to opt-out of certain data processing activities
                </li>
              </ul>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">7. Contact Us</h2>
              <p className="text-muted-foreground">
                If you have questions about this Privacy Policy or our privacy
                practices, please contact us at:
              </p>
              <p className="text-muted-foreground mt-3">
                <strong className="text-foreground">mycel</strong>
                <br />
                Email:{" "}
                <a
                  href="mailto:puneet@mycel.dev"
                  className="text-primary hover:underline"
                >
                  puneet@mycel.dev
                </a>
              </p>
            </div>

            <div className="pt-8 border-t border-border">
              <p className="text-sm text-muted-foreground">
                This Privacy Policy is subject to change without notice. We will
                notify you of significant changes by posting a notice on our
                website.
              </p>
            </div>
          </section>
        </article>
      </div>

      <Footer />
    </main>
  );
}
